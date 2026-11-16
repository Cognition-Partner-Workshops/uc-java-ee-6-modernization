package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	daytradergo "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/javafmt"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/seed"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// App carries the dependencies every handler gets (constructor injection instead of JNDI).
type App struct {
	Config    *config.Config
	Store     *store.Store
	Sessions  *SessionStore
	Log       *slog.Logger
	templates *template.Template
}

// TemplateFuncs are available in every template.
var TemplateFuncs = template.FuncMap{
	"date":            javafmt.Date,
	"double":          javafmt.Double,
	"money":           javafmt.Money,
	"gainHTML":        javafmt.PrintGainHTML,
	"changeHTML":      javafmt.PrintChangeHTML,
	"gainPercentHTML": javafmt.PrintGainPercentHTML,
	"quoteLink":       javafmt.PrintQuoteLink,
	"now":             func() string { return javafmt.Date(time.Now()) },
	"safeHTML":        func(text string) template.HTML { return template.HTML(text) },
	"computeGain":     javafmt.ComputeGain,
	"computeGainPct":  javafmt.ComputeGainPercent,
	"derefTime":       func(t *time.Time) time.Time { return *t },
	"formatNullableTS": func(t *time.Time) string {
		if t == nil {
			return "null"
		}
		return javafmt.Date(*t)
	},
}

// New parses every templates/*.html file into one set and returns the App.
func New(cfg *config.Config, db *store.Store, logger *slog.Logger) (*App, error) {
	templates, err := template.New("daytrader").Funcs(TemplateFuncs).ParseFS(daytradergo.Templates, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &App{Config: cfg, Store: db, Sessions: NewSessionStore(cfg.ContextPath), Log: logger, templates: templates}, nil
}

// Render executes a named template into a buffer first, so a template error becomes a clean
// 500 instead of half a page. Content type is text/html like the legacy servlet.
func (a *App) Render(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	var buffer bytes.Buffer
	if err := a.templates.ExecuteTemplate(&buffer, name, data); err != nil {
		a.Error(w, r, fmt.Errorf("render %s: %w", name, err))
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}

// Welcome shows the welcome/login page with a status line (legacy doWelcome).
func (a *App) Welcome(w http.ResponseWriter, r *http.Request, status string) {
	registry.Lock()
	handler := registry.welcome
	registry.Unlock()
	if handler != nil {
		handler(a, w, r, status)
		return
	}
	a.Render(w, r, http.StatusOK, "scaffold_welcome", map[string]any{"Results": status, "Actions": RegisteredActions()})
}

// Error answers HTTP 500 (legacy: ServletException -> error.jsp).
func (a *App) Error(w http.ResponseWriter, r *http.Request, err error) {
	a.Log.Error("request failed", "path", r.URL.Path, "action", r.FormValue("action"), "error", err)
	registry.Lock()
	handler := registry.errPage
	registry.Unlock()
	if handler != nil {
		handler(a, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	_ = a.templates.ExecuteTemplate(w, "scaffold_error", map[string]any{"Message": err.Error()})
}

// ClosedOrders is the per-page "orders have completed" lookup. It never fails the page.
func (a *App) ClosedOrders(ctx context.Context, userID string) []*store.Order {
	orders, err := a.Store.ClosedOrders(ctx, userID)
	if err != nil {
		a.Log.Error("closed orders", "user", userID, "error", err)
	}
	return orders
}

// frontController is TradeAppServlet.performTask: dispatch on the action parameter.
func (a *App) frontController(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !r.URL.Query().Has("action") && r.PostFormValue("action") == "" {
		a.Welcome(w, r, "")
		return
	}
	name := r.FormValue("action")
	registry.Lock()
	entry, found := registry.actions[name]
	registry.Unlock()
	if found && entry.public {
		entry.handler(a, w, r, a.Sessions.Get(w, r))
		return
	}
	session := a.Sessions.Get(w, r)
	if session.UserID == "" {
		a.Welcome(w, r, "User Not Logged in")
		return
	}
	if !found {
		a.Welcome(w, r, "TradeAppServlet: Invalid Action"+name)
		return
	}
	entry.handler(a, w, r, session)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	body := map[string]any{"status": "UP", "database": "UP", "orderProcessingMode": a.Config.OrderProcessingMode}
	status := http.StatusOK
	if err := a.Store.Pool.Ping(ctx); err != nil {
		body["status"], body["database"] = "DOWN", "DOWN"
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// config is the slice of the legacy TradeConfigServlet the parity seeder uses:
// /config?action=buildDBTables and /config?action=buildDB.
func (a *App) config(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	switch r.FormValue("action") {
	case "buildDBTables":
		if err := seed.BuildTables(r.Context(), a.Store); err != nil {
			a.Error(w, r, err)
			return
		}
		fmt.Fprint(w, "<HTML><BODY><BR>TradeBuildDB: **** Dropping and Recreating the DayTrader tables... ****</BR>"+
			"<BR>DayTrader tables successfully created!</BR><BR>Please Stop and Re-start your Daytrader application</BR></BODY></HTML>")
	case "buildDB":
		result, err := seed.BuildDB(r.Context(), a.Store, a.Config, nil)
		if err != nil {
			a.Error(w, r, err)
			return
		}
		fmt.Fprintf(w, "<HTML><BODY><BR>TradeBuildDB: **** Creating %d Quotes ****</BR><BR>**** Registering %d Users **** </BR>"+
			"<BR>%d holdings, %d orders</BR></BODY></HTML>", result.Quotes, result.Users, result.Holdings, result.Orders)
	default:
		http.NotFound(w, r)
	}
}

// Handler builds the mux: the front controller, health, config, registered routes and the
// legacy static files, all under the context path; REST routes under the REST root.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	base := a.Config.ContextPath
	mux.HandleFunc(base+"/app", a.frontController)
	mux.HandleFunc("GET "+base+"/health", a.health)
	mux.HandleFunc("GET "+base+"/config", a.config)
	registry.Lock()
	routes := append([]route(nil), registry.routes...)
	registry.Unlock()
	for _, entry := range routes {
		prefix := base
		if entry.rest {
			prefix = a.Config.RestPath
		}
		method, path, hasMethod := strings.Cut(entry.pattern, " ")
		if !hasMethod {
			method, path = "", entry.pattern
		} else {
			method += " "
		}
		mux.Handle(method+prefix+path, entry.build(a))
	}
	staticFiles, err := fs.Sub(daytradergo.Static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle(base+"/", http.StripPrefix(base+"/", http.FileServerFS(staticFiles)))
	mux.Handle("/{$}", http.RedirectHandler(base+"/", http.StatusFound))
	return a.accessLog(mux)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *App) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				a.Error(writer, r, fmt.Errorf("panic: %v", recovered))
			}
			a.Log.Info("request", "method", r.Method, "path", r.URL.Path, "action", r.URL.Query().Get("action"),
				"status", writer.status, "ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(writer, r)
	})
}
