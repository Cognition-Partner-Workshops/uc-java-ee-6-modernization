package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/seed"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// resetRegistry gives the test an empty registry and puts the old one back afterwards.
func resetRegistry(t *testing.T) {
	t.Helper()
	registry.Lock()
	actions, routes, welcome, errPage := registry.actions, registry.routes, registry.welcome, registry.errPage
	registry.actions, registry.routes, registry.welcome, registry.errPage = map[string]action{}, nil, nil, nil
	registry.Unlock()
	t.Cleanup(func() {
		registry.Lock()
		registry.actions, registry.routes, registry.welcome, registry.errPage = actions, routes, welcome, errPage
		registry.Unlock()
	})
}

func testConfig(t *testing.T, values map[string]string) *config.Config {
	t.Helper()
	cfg, err := config.FromEnv(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// newTestApp builds an App with an empty registry. Its pool points at a closed port and never
// connects unless a test uses the database, so these tests need no PostgreSQL.
func newTestApp(t *testing.T) *App {
	t.Helper()
	resetRegistry(t)
	pool, err := pgxpool.New(context.Background(), "postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	app, err := New(testConfig(t, nil), store.New(pool, 10), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

type response struct {
	status  int
	body    string
	header  http.Header
	cookies []*http.Cookie
}

func do(t *testing.T, handler http.Handler, method, target string, form url.Values, cookies ...*http.Cookie) response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequest(method, target, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	result := recorder.Result()
	text, _ := io.ReadAll(result.Body)
	return response{status: result.StatusCode, body: string(text), header: result.Header, cookies: result.Cookies()}
}

func sessionCookieOf(t *testing.T, r response) *http.Cookie {
	t.Helper()
	for _, cookie := range r.cookies {
		if cookie.Name == "JSESSIONID" {
			return cookie
		}
	}
	t.Fatal("no JSESSIONID cookie")
	return nil
}

// registerLogin adds a public "login" action that signs the uid parameter in, like the auth area.
func registerLogin() {
	RegisterPublicAction("login", func(app *App, w http.ResponseWriter, r *http.Request, session *Session) {
		session.UserID = Param(r, "uid")
		session.CreationDate = time.Now()
		_, _ = io.WriteString(w, "logged in "+session.UserID)
	})
}

func login(t *testing.T, handler http.Handler, userID string) *http.Cookie {
	t.Helper()
	return sessionCookieOf(t, do(t, handler, http.MethodGet, "/daytrader/app?action=login&uid="+url.QueryEscape(userID), nil))
}

func TestFrontControllerWithoutActionShowsTheWelcomePage(t *testing.T) {
	handler := newTestApp(t).Handler()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		r := do(t, handler, method, "/daytrader/app", nil)
		if r.status != http.StatusOK || r.header.Get("Content-Type") != "text/html" {
			t.Errorf("%s: status %d, content type %q", method, r.status, r.header.Get("Content-Type"))
		}
		if !strings.Contains(r.body, "<TITLE>DayTrader Login</TITLE>") || !strings.Contains(r.body, `<FONT color="#ff0000"></FONT>`) {
			t.Errorf("%s: body = %s", method, r.body)
		}
	}
}

func TestProtectedActionWithoutLoginShowsUserNotLoggedIn(t *testing.T) {
	app := newTestApp(t)
	called := false
	RegisterAction("home", func(*App, http.ResponseWriter, *http.Request, *Session) { called = true })
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=home", nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "User Not Logged in") || called {
		t.Errorf("status %d, called %v, body %s", r.status, called, r.body)
	}
	// An unknown action without a login gets the same answer, not "Invalid Action".
	r = do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=bogus", nil)
	if !strings.Contains(r.body, "User Not Logged in") || strings.Contains(r.body, "Invalid Action") {
		t.Errorf("body %s", r.body)
	}
	// An empty action parameter is an action, not the plain welcome page.
	r = do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=", nil)
	if !strings.Contains(r.body, "User Not Logged in") {
		t.Errorf("body %s", r.body)
	}
}

func TestPublicActionRunsWithoutLoginAndGetsASessionCookie(t *testing.T) {
	app := newTestApp(t)
	var seen *Session
	RegisterPublicAction("register", func(_ *App, w http.ResponseWriter, _ *http.Request, session *Session) {
		seen = session
		_, _ = io.WriteString(w, "registered")
	})
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=register", nil)
	if r.status != http.StatusOK || r.body != "registered" {
		t.Fatalf("status %d body %q", r.status, r.body)
	}
	if seen == nil || seen.UserID != "" || seen.Values == nil {
		t.Fatalf("session = %+v", seen)
	}
	cookie := sessionCookieOf(t, r)
	if cookie.Value != seen.ID || len(cookie.Value) != 32 || cookie.Path != "/daytrader" || !cookie.HttpOnly {
		t.Errorf("cookie = %+v, session id %q", cookie, seen.ID)
	}
}

func TestLoginThenProtectedActionUsesTheSameSession(t *testing.T) {
	app := newTestApp(t)
	registerLogin()
	RegisterAction("home", func(_ *App, w http.ResponseWriter, _ *http.Request, session *Session) {
		_, _ = io.WriteString(w, "home of "+session.UserID)
	})
	handler := app.Handler()
	cookie := login(t, handler, "uid:0")
	r := do(t, handler, http.MethodGet, "/daytrader/app?action=home", nil, cookie)
	if r.body != "home of uid:0" {
		t.Errorf("body %q", r.body)
	}
	if len(r.cookies) != 0 {
		t.Errorf("an existing session must not get a new cookie: %v", r.cookies)
	}
	// Legacy message: "TradeAppServlet: Invalid Action" + action, without a space.
	r = do(t, handler, http.MethodGet, "/daytrader/app?action=bogus", nil, cookie)
	if r.status != http.StatusOK || !strings.Contains(r.body, "TradeAppServlet: Invalid Actionbogus") {
		t.Errorf("status %d body %s", r.status, r.body)
	}
	// A cookie the server does not know is a new visitor.
	r = do(t, handler, http.MethodGet, "/daytrader/app?action=home", nil, &http.Cookie{Name: "JSESSIONID", Value: "unknown"})
	if !strings.Contains(r.body, "User Not Logged in") || sessionCookieOf(t, r).Value == "unknown" {
		t.Errorf("body %s", r.body)
	}
}

func TestActionIsReadFromThePostedForm(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("login", func(_ *App, w http.ResponseWriter, r *http.Request, _ *Session) {
		_, _ = io.WriteString(w, "uid="+r.FormValue("uid")+" passwd="+Param(r, "passwd"))
	})
	r := do(t, app.Handler(), http.MethodPost, "/daytrader/app", url.Values{"action": {"login"}, "uid": {"uid:0"}, "passwd": {"xxx"}})
	if r.body != "uid=uid:0 passwd=xxx" {
		t.Errorf("body %q", r.body)
	}
}

func TestFrontControllerRejectsOtherMethods(t *testing.T) {
	handler := newTestApp(t).Handler()
	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if r := do(t, handler, method, "/daytrader/app?action=home", nil); r.status != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d", method, r.status)
		}
	}
}

func TestRegisteringAnActionTwicePanics(t *testing.T) {
	newTestApp(t)
	noop := func(*App, http.ResponseWriter, *http.Request, *Session) {}
	RegisterAction("home", noop)
	for name, again := range map[string]func(){
		"RegisterAction":       func() { RegisterAction("home", noop) },
		"RegisterPublicAction": func() { RegisterPublicAction("home", noop) },
	} {
		func() {
			defer func() {
				if recovered := recover(); recovered == nil || !strings.Contains(recovered.(string), `"home" registered twice`) {
					t.Errorf("%s: recovered %v", name, recovered)
				}
			}()
			again()
		}()
	}
}

func TestRegisteredActionsAreSorted(t *testing.T) {
	newTestApp(t)
	if got := RegisteredActions(); len(got) != 0 {
		t.Fatalf("empty registry lists %v", got)
	}
	noop := func(*App, http.ResponseWriter, *http.Request, *Session) {}
	RegisterAction("quotes", noop)
	RegisterPublicAction("login", noop)
	RegisterAction("home", noop)
	if got := strings.Join(RegisteredActions(), ","); got != "home,login,quotes" {
		t.Errorf("got %s", got)
	}
}

func TestScaffoldWelcomeListsTheRegisteredActions(t *testing.T) {
	app := newTestApp(t)
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app", nil)
	if !strings.Contains(r.body, "Registered actions: none") {
		t.Errorf("body %s", r.body)
	}
	RegisterAction("home", func(*App, http.ResponseWriter, *http.Request, *Session) {})
	r = do(t, app.Handler(), http.MethodGet, "/daytrader/app", nil)
	if !strings.Contains(r.body, "<CODE>home</CODE>") {
		t.Errorf("body %s", r.body)
	}
}

func TestRegisterWelcomeReplacesTheWelcomePage(t *testing.T) {
	app := newTestApp(t)
	RegisterWelcome(func(_ *App, w http.ResponseWriter, _ *http.Request, status string) {
		_, _ = io.WriteString(w, "welcome["+status+"]")
	})
	handler := app.Handler()
	for target, want := range map[string]string{
		"/daytrader/app":             "welcome[]",
		"/daytrader/app?action=home": "welcome[User Not Logged in]",
	} {
		if r := do(t, handler, http.MethodGet, target, nil); r.body != want {
			t.Errorf("%s: body %q, want %q", target, r.body, want)
		}
	}
}

func TestErrorAnswers500(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("boom", func(app *App, w http.ResponseWriter, r *http.Request, _ *Session) {
		app.Error(w, r, errors.New("<boom> happened"))
	})
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=boom", nil)
	if r.status != http.StatusInternalServerError || r.header.Get("Content-Type") != "text/html" {
		t.Errorf("status %d content type %q", r.status, r.header.Get("Content-Type"))
	}
	if !strings.Contains(r.body, "An Error has occurred during DayTrader processing.") || !strings.Contains(r.body, "&lt;boom&gt; happened") {
		t.Errorf("body %s", r.body)
	}
	RegisterErrorPage(func(_ *App, w http.ResponseWriter, _ *http.Request, err error) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "custom: "+err.Error())
	})
	r = do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=boom", nil)
	if r.status != http.StatusInternalServerError || r.body != "custom: <boom> happened" {
		t.Errorf("status %d body %q", r.status, r.body)
	}
}

func TestPanicInAHandlerAnswers500(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("panic", func(*App, http.ResponseWriter, *http.Request, *Session) { panic("kaboom") })
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=panic", nil)
	if r.status != http.StatusInternalServerError || !strings.Contains(r.body, "panic: kaboom") {
		t.Errorf("status %d body %s", r.status, r.body)
	}
}

func TestRenderUnknownTemplateIsAClean500(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("page", func(app *App, w http.ResponseWriter, r *http.Request, _ *Session) {
		app.Render(w, r, http.StatusOK, "tests_missing_template", nil)
	})
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=page", nil)
	if r.status != http.StatusInternalServerError || !strings.Contains(r.body, "render tests_missing_template") {
		t.Errorf("status %d body %s", r.status, r.body)
	}
}

func TestRenderUsesTheGivenStatus(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	app.Render(recorder, httptest.NewRequest(http.MethodGet, "/daytrader/app", nil), http.StatusAccepted, "scaffold_welcome",
		map[string]any{"Results": "<b>", "Actions": []string{}})
	if recorder.Code != http.StatusAccepted || recorder.Header().Get("Content-Type") != "text/html" ||
		!strings.Contains(recorder.Body.String(), "&lt;b&gt;") {
		t.Errorf("status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestForwardHandsOverWithAttributes(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("register", func(app *App, w http.ResponseWriter, r *http.Request, session *Session) {
		app.Forward(w, r, session, "login", map[string]string{"uid": "uid:new", "results": "Registration operation succeeded"})
	})
	RegisterPublicAction("login", func(app *App, w http.ResponseWriter, r *http.Request, session *Session) {
		session.UserID = Param(r, "uid")
		app.Forward(w, r, session, "home", map[string]string{"results": "Ready to Trade"})
	})
	RegisterAction("home", func(_ *App, w http.ResponseWriter, r *http.Request, session *Session) {
		results, hasResults := Attr(r, "results")
		_, hasOther := Attr(r, "other")
		// uid: the attribute wins over the form value; passwd: no attribute, so the form value;
		// results: the later forward wins; attributes of earlier forwards are kept.
		_, _ = io.WriteString(w, strings.Join([]string{session.UserID, Param(r, "uid"), Param(r, "passwd"), results}, "|"))
		if !hasResults || hasOther {
			_, _ = io.WriteString(w, "|unexpected attributes")
		}
	})
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=register&uid=uid:form&passwd=secret", nil)
	if r.body != "uid:new|uid:new|secret|Ready to Trade" {
		t.Errorf("body %q", r.body)
	}
}

func TestForwardToAnUnregisteredActionShowsTheWelcomePage(t *testing.T) {
	app := newTestApp(t)
	RegisterPublicAction("login", func(app *App, w http.ResponseWriter, r *http.Request, session *Session) {
		app.Forward(w, r, session, "home", nil)
	})
	r := do(t, app.Handler(), http.MethodGet, "/daytrader/app?action=login", nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "TradeAppServlet: Invalid Actionhome") {
		t.Errorf("status %d body %s", r.status, r.body)
	}
}

func TestAttrAndParamWithoutForward(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/daytrader/app?uid=uid:1", nil)
	if value, ok := Attr(request, "uid"); ok || value != "" {
		t.Errorf("Attr = %q, %v", value, ok)
	}
	if Param(request, "uid") != "uid:1" || Param(request, "missing") != "" {
		t.Errorf("Param = %q, %q", Param(request, "uid"), Param(request, "missing"))
	}
}

func TestRoutesAreMountedUnderTheContextPathAndTheRestRoot(t *testing.T) {
	app := newTestApp(t)
	RegisterRoute("GET /welcome.jsp", func(app *App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "welcome under "+app.Config.ContextPath)
		})
	})
	RegisterRoute("/register.jsp", func(*App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "register "+r.Method) })
	})
	RegisterRestRoute("GET /addresses/{entryName}", func(*App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "address "+r.PathValue("entryName"))
		})
	})
	handler := app.Handler()
	checks := []struct {
		method, target string
		status         int
		body           string
	}{
		{http.MethodGet, "/daytrader/welcome.jsp", http.StatusOK, "welcome under /daytrader"},
		// A method the route does not take falls through to the static files: 404.
		{http.MethodPost, "/daytrader/welcome.jsp", http.StatusNotFound, ""},
		{http.MethodPost, "/daytrader/register.jsp", http.StatusOK, "register POST"},
		{http.MethodGet, "/rest/addresses/uid:0", http.StatusOK, "address uid:0"},
		{http.MethodGet, "/daytrader/addresses/uid:0", http.StatusNotFound, ""},
		{http.MethodGet, "/rest/welcome.jsp", http.StatusNotFound, ""},
	}
	for _, check := range checks {
		r := do(t, handler, check.method, check.target, nil)
		if r.status != check.status || check.body != "" && r.body != check.body {
			t.Errorf("%s %s: status %d body %q", check.method, check.target, r.status, r.body)
		}
	}
}

func TestContextPathAndRestPathAreConfigurable(t *testing.T) {
	app := newTestApp(t)
	app.Config = testConfig(t, map[string]string{"CONTEXT_PATH": "/trade", "REST_PATH": "/api"})
	RegisterRestRoute("GET /addresses", func(*App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "addresses") })
	})
	handler := app.Handler()
	if r := do(t, handler, http.MethodGet, "/trade/app", nil); r.status != http.StatusOK || !strings.Contains(r.body, "DayTrader Login") {
		t.Errorf("/trade/app: status %d", r.status)
	}
	if r := do(t, handler, http.MethodGet, "/api/addresses", nil); r.body != "addresses" {
		t.Errorf("/api/addresses: status %d body %q", r.status, r.body)
	}
	if r := do(t, handler, http.MethodGet, "/daytrader/app", nil); r.status != http.StatusNotFound {
		t.Errorf("/daytrader/app: status %d", r.status)
	}
	if r := do(t, handler, http.MethodGet, "/", nil); r.header.Get("Location") != "/trade/" {
		t.Errorf("/ redirects to %q", r.header.Get("Location"))
	}
}

func TestStaticFilesAndRootRedirect(t *testing.T) {
	handler := newTestApp(t).Handler()
	r := do(t, handler, http.MethodGet, "/daytrader/style.css", nil)
	if r.status != http.StatusOK || !strings.HasPrefix(r.header.Get("Content-Type"), "text/css") || len(r.body) == 0 {
		t.Errorf("style.css: status %d content type %q", r.status, r.header.Get("Content-Type"))
	}
	r = do(t, handler, http.MethodGet, "/daytrader/", nil)
	if r.status != http.StatusOK || !strings.HasPrefix(r.header.Get("Content-Type"), "text/html") {
		t.Errorf("/daytrader/: status %d content type %q", r.status, r.header.Get("Content-Type"))
	}
	if r = do(t, handler, http.MethodGet, "/daytrader/nothing.html", nil); r.status != http.StatusNotFound {
		t.Errorf("nothing.html: status %d", r.status)
	}
	r = do(t, handler, http.MethodGet, "/", nil)
	if r.status != http.StatusFound || r.header.Get("Location") != "/daytrader/" {
		t.Errorf("/: status %d location %q", r.status, r.header.Get("Location"))
	}
}

func TestHealthIsDownWithoutADatabase(t *testing.T) {
	r := do(t, newTestApp(t).Handler(), http.MethodGet, "/daytrader/health", nil)
	if r.status != http.StatusServiceUnavailable || r.header.Get("Content-Type") != "application/json" {
		t.Fatalf("status %d content type %q", r.status, r.header.Get("Content-Type"))
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "DOWN" || body["database"] != "DOWN" || body["orderProcessingMode"] != "sync" {
		t.Errorf("body %v", body)
	}
}

func TestConfigEndpointRejectsUnknownActionsAndPost(t *testing.T) {
	handler := newTestApp(t).Handler()
	if r := do(t, handler, http.MethodGet, "/daytrader/config?action=resetTrade", nil); r.status != http.StatusNotFound {
		t.Errorf("unknown action: status %d", r.status)
	}
	if r := do(t, handler, http.MethodGet, "/daytrader/config", nil); r.status != http.StatusNotFound {
		t.Errorf("no action: status %d", r.status)
	}
	// Without a database buildDBTables is a 500, not a hang or a 200.
	if r := do(t, handler, http.MethodGet, "/daytrader/config?action=buildDBTables", nil); r.status != http.StatusInternalServerError {
		t.Errorf("buildDBTables without a database: status %d", r.status)
	}
}

func TestInvalidateDropsTheSessionAndExpiresTheCookie(t *testing.T) {
	app := newTestApp(t)
	registerLogin()
	RegisterAction("logout", func(app *App, w http.ResponseWriter, _ *http.Request, session *Session) {
		app.Sessions.Invalidate(w, session)
		_, _ = io.WriteString(w, "bye")
	})
	handler := app.Handler()
	cookie := login(t, handler, "uid:0")
	r := do(t, handler, http.MethodGet, "/daytrader/app?action=logout", nil, cookie)
	expired := sessionCookieOf(t, r)
	if r.body != "bye" || expired.MaxAge >= 0 || expired.Value != "" || expired.Path != "/daytrader" {
		t.Errorf("body %q cookie %+v", r.body, expired)
	}
	r = do(t, handler, http.MethodGet, "/daytrader/app?action=logout", nil, cookie)
	if !strings.Contains(r.body, "User Not Logged in") {
		t.Errorf("the old cookie still works: %s", r.body)
	}
}

func TestSessionsExpireAfterThirtyIdleMinutes(t *testing.T) {
	sessions := NewSessionStore("/daytrader")
	first := httptest.NewRecorder()
	session := sessions.Get(first, httptest.NewRequest(http.MethodGet, "/daytrader/app", nil))
	session.UserID = "uid:0"
	cookie := first.Result().Cookies()[0]
	withCookie := func() *http.Request {
		request := httptest.NewRequest(http.MethodGet, "/daytrader/app", nil)
		request.AddCookie(cookie)
		return request
	}
	session.lastSeen = time.Now().Add(-29 * time.Minute)
	if again := sessions.Get(httptest.NewRecorder(), withCookie()); again != session || time.Since(again.lastSeen) > time.Minute {
		t.Fatalf("a session idle for 29 minutes was replaced or not touched: %+v", again)
	}
	session.lastSeen = time.Now().Add(-31 * time.Minute)
	recorder := httptest.NewRecorder()
	fresh := sessions.Get(recorder, withCookie())
	if fresh == session || fresh.UserID != "" || fresh.ID == session.ID || len(recorder.Result().Cookies()) != 1 {
		t.Errorf("a session idle for 31 minutes is still in use: %+v", fresh)
	}
	// Idle sessions of other visitors are swept when a new session is created.
	fresh.lastSeen = time.Now().Add(-time.Hour)
	sessions.Get(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/daytrader/app", nil))
	sessions.mutex.Lock()
	_, stillThere := sessions.sessions[fresh.ID]
	remaining := len(sessions.sessions)
	sessions.mutex.Unlock()
	if stillThere || remaining != 1 {
		t.Errorf("idle session kept: %v, %d sessions", stillThere, remaining)
	}
}

func TestNewPageWithoutALoginDoesNotTouchTheDatabase(t *testing.T) {
	app := newTestApp(t)
	request := httptest.NewRequest(http.MethodGet, "/daytrader/app", nil)
	for _, session := range []*Session{nil, {}} {
		page := app.NewPage(request, session, "Welcome to DayTrader", "DayTrader Home")
		if page.Title != "Welcome to DayTrader" || page.Banner != "DayTrader Home" || page.ClosedOrders != nil {
			t.Errorf("page = %+v", page)
		}
	}
}

// The closed-orders lookup never fails the page: with the database down the page renders without the alert.
func TestNewPageSurvivesADatabaseError(t *testing.T) {
	app := newTestApp(t)
	page := app.NewPage(httptest.NewRequest(http.MethodGet, "/daytrader/app", nil), &Session{UserID: "uid:0"}, "t", "b")
	if len(page.ClosedOrders) != 0 {
		t.Errorf("closed orders = %v", page.ClosedOrders)
	}
}

func renderLayout(t *testing.T, app *App, page Page) string {
	t.Helper()
	var text strings.Builder
	for _, name := range []string{"pageStart", "pageEnd"} {
		if err := app.templates.ExecuteTemplate(&text, name, page); err != nil {
			t.Fatal(err)
		}
	}
	return text.String()
}

func TestLayoutRendersTitleBannerAndNavigation(t *testing.T) {
	app := newTestApp(t)
	html := renderLayout(t, app, Page{Title: "Welcome to DayTrader", Banner: "DayTrader Home"})
	for _, want := range []string{
		"<TITLE>Welcome to DayTrader</TITLE>",
		`color="#ffffff">DayTrader Home</FONT></B></TD>`,
		`<A href="app?action=home">Home</A>`, `<A href="app?action=account">Account</A>`, `<A href="app?action=portfolio">Portfolio</A>`,
		`<A href="app?action=quotes&amp;symbols=s:0,s:1,s:2,s:3,s:4">Quotes/Trade</A>`, `<A href="app?action=logout">Logoff</A>`,
		`value="s:0, s:1, s:2, s:3, s:4"></FORM>`, "</HTML>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("layout lacks %q", want)
		}
	}
	if strings.Count(html, "DayTrader Home") != 2 {
		t.Errorf("the banner must be at the top and the bottom")
	}
	if strings.Contains(html, "Alert:") {
		t.Error("alert shown without closed orders")
	}
}

func TestLayoutShowsTheClosedOrdersAlert(t *testing.T) {
	app := newTestApp(t)
	opened := time.Date(2026, time.November, 4, 7, 42, 47, 0, time.UTC)
	completed := opened.Add(time.Second)
	orders := []*store.Order{
		{OrderID: 15001, OrderType: "buy", OrderStatus: "completed", OpenDate: opened, CompletionDate: &completed, Quantity: 10,
			Price: decimal.RequireFromString("145.06"), OrderFee: decimal.RequireFromString("24.95"), Symbol: "s:1"},
		{OrderID: 15002, OrderType: "sell", OrderStatus: "completed", OpenDate: opened, Quantity: 2.5,
			OrderFee: decimal.RequireFromString("24.95"), Symbol: "s:2"},
	}
	html := renderLayout(t, app, Page{Title: "t", Banner: "b", ClosedOrders: orders})
	for _, want := range []string{
		"The following Order(s) have completed.",
		"<TD>15001</TD>", "<TD>completed</TD>", "<TD>Wed Nov 04 07:42:47 UTC 2026</TD>", "<TD>Wed Nov 04 07:42:48 UTC 2026</TD>",
		"<TD>24.95</TD>", "<TD>buy</TD>", `<TD><A href="app?action=quotes&symbols=s:1">s:1</A></TD>`, "<TD>10.0</TD>",
		"<TD>15002</TD>", "<TD>null</TD>", "<TD>sell</TD>", "<TD>2.5</TD>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("alert lacks %q", want)
		}
	}
}

func TestTemplateFuncs(t *testing.T) {
	for _, name := range []string{"date", "double", "money", "gainHTML", "changeHTML", "gainPercentHTML", "quoteLink", "now", "safeHTML",
		"computeGain", "computeGainPct", "formatNullableTS"} {
		if TemplateFuncs[name] == nil {
			t.Errorf("template function %q is documented in the README but missing", name)
		}
	}
	formatNullable := TemplateFuncs["formatNullableTS"].(func(*time.Time) string)
	at := time.Date(2026, time.November, 4, 7, 42, 47, 0, time.UTC)
	if formatNullable(nil) != "null" || formatNullable(&at) != "Wed Nov 04 07:42:47 UTC 2026" {
		t.Errorf("formatNullableTS: %q, %q", formatNullable(nil), formatNullable(&at))
	}
	now := TemplateFuncs["now"].(func() string)()
	if _, err := time.Parse("Mon Jan 02 15:04:05 MST 2006", now); err != nil || !strings.Contains(now, " UTC ") {
		t.Errorf("now = %q: %v", now, err)
	}
}

// ---- database tests (skipped without TEST_DATABASE_URL) ----

func newDatabaseApp(t *testing.T) *App {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; start PostgreSQL with `make go-db` to run the database tests")
	}
	const schema = "daytrader_test_web"
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	_, err = admin.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema)
	_ = admin.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	cfg := testConfig(t, map[string]string{"MAX_USERS": "6", "MAX_QUOTES": "8", "MAX_HOLDINGS": "3", "KEY_BLOCK_SIZE": "10"})
	db, err := store.Open(ctx, parsed.String(), 4, cfg.KeyBlockSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	resetRegistry(t)
	app, err := New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestHealthIsUpWithADatabase(t *testing.T) {
	r := do(t, newDatabaseApp(t).Handler(), http.MethodGet, "/daytrader/health", nil)
	var body map[string]string
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatal(err)
	}
	if r.status != http.StatusOK || body["status"] != "UP" || body["database"] != "UP" || body["orderProcessingMode"] != "sync" {
		t.Errorf("status %d body %v", r.status, body)
	}
}

func TestConfigEndpointBuildsTheTablesAndTheDatabase(t *testing.T) {
	app := newDatabaseApp(t)
	handler := app.Handler()
	r := do(t, handler, http.MethodGet, "/daytrader/config?action=buildDBTables", nil)
	if r.status != http.StatusOK || r.header.Get("Content-Type") != "text/html" || !strings.Contains(r.body, "DayTrader tables successfully created!") {
		t.Fatalf("buildDBTables: status %d body %s", r.status, r.body)
	}
	r = do(t, handler, http.MethodGet, "/daytrader/config?action=buildDB", nil)
	if r.status != http.StatusOK || !strings.Contains(r.body, "**** Creating 8 Quotes ****") || !strings.Contains(r.body, "**** Registering 6 Users ****") {
		t.Fatalf("buildDB: status %d body %s", r.status, r.body)
	}
	account, err := store.AccountByUser(context.Background(), app.Store.Pool, "uid:0")
	if err != nil || account.OpenBalance.StringFixed(2) != "1000000.00" {
		t.Errorf("uid:0 = %+v, %v", account, err)
	}
}

// Every signed-in page shows the user's closed orders once: the seeded buys of uid:<n> are
// "closed", the first page after login alerts them and marks them completed.
func TestSignedInPageAlertsClosedOrdersOnce(t *testing.T) {
	app := newDatabaseApp(t)
	ctx := context.Background()
	if err := seed.BuildTables(ctx, app.Store); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Pool.Exec(ctx, `INSERT INTO accountejb (accountid, logincount, logoutcount, lastlogin, creationdate, balance, openbalance, profile_userid)
		VALUES (1, 0, 0, now(), now(), 10.00, 10.00, 'uid:a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Pool.Exec(ctx, `INSERT INTO orderejb (orderid, ordertype, orderstatus, opendate, completiondate, quantity, price, orderfee, account_accountid, quote_symbol, holding_holdingid)
		VALUES (7, 'buy', 'closed', '2026-11-04 07:42:47', '2026-11-04 07:42:48', 10, 145.06, 24.95, 1, 's:1', NULL)`); err != nil {
		t.Fatal(err)
	}
	registerLogin()
	RegisterAction("home", func(app *App, w http.ResponseWriter, r *http.Request, session *Session) {
		page := app.NewPage(r, session, "Welcome to DayTrader", "DayTrader Home")
		w.Header().Set("Content-Type", "text/html")
		_ = app.templates.ExecuteTemplate(w, "pageStart", page)
		_ = app.templates.ExecuteTemplate(w, "pageEnd", page)
	})
	handler := app.Handler()
	cookie := login(t, handler, "uid:a")
	first := do(t, handler, http.MethodGet, "/daytrader/app?action=home", nil, cookie)
	for _, want := range []string{"The following Order(s) have completed.", "<TD>7</TD>", "<TD>completed</TD>",
		"<TD>Wed Nov 04 07:42:47 UTC 2026</TD>", "<TD>Wed Nov 04 07:42:48 UTC 2026</TD>", "<TD>10.0</TD>"} {
		if !strings.Contains(first.body, want) {
			t.Errorf("first page lacks %q", want)
		}
	}
	second := do(t, handler, http.MethodGet, "/daytrader/app?action=home", nil, cookie)
	if second.status != http.StatusOK || strings.Contains(second.body, "Alert:") {
		t.Errorf("the alert is shown twice")
	}
}
