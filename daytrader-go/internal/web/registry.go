// Package web is the shared HTTP layer: the /app?action=... front controller, the handler
// registry, sessions and template rendering. Feature areas register their handlers from
// init() in their own package (internal/features/<area>) and never edit this package.
package web

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// ActionFunc handles one /app?action=<name> request. session is never nil; session.UserID is
// empty for a visitor who is not logged in (only possible for public actions).
type ActionFunc func(app *App, w http.ResponseWriter, r *http.Request, session *Session)

// WelcomeFunc renders the welcome/login page with a status line (legacy doWelcome).
type WelcomeFunc func(app *App, w http.ResponseWriter, r *http.Request, status string)

// ErrorFunc renders the HTTP 500 page (legacy error.jsp).
type ErrorFunc func(app *App, w http.ResponseWriter, r *http.Request, err error)

// RouteFunc builds a handler once the App exists.
type RouteFunc func(app *App) http.Handler

type action struct {
	handler ActionFunc
	public  bool
}

type route struct {
	pattern string
	build   RouteFunc
	rest    bool
}

var registry = struct {
	sync.Mutex
	actions map[string]action
	routes  []route
	welcome WelcomeFunc
	errPage ErrorFunc
}{actions: map[string]action{}}

func register(name string, entry action) {
	registry.Lock()
	defer registry.Unlock()
	if _, exists := registry.actions[name]; exists {
		panic(fmt.Sprintf("web: action %q registered twice", name))
	}
	registry.actions[name] = entry
}

// RegisterAction adds a handler for /app?action=<name> that requires a logged-in session.
// A request without one gets the welcome page with "User Not Logged in", as in TradeAppServlet.
func RegisterAction(name string, handler ActionFunc) {
	register(name, action{handler: handler})
}

// RegisterPublicAction adds a handler that runs without a login (legacy: login, register).
func RegisterPublicAction(name string, handler ActionFunc) {
	register(name, action{handler: handler, public: true})
}

// RegisterWelcome sets the page shown for /app without an action, for a missing login and for
// an unknown action. Owned by the auth area.
func RegisterWelcome(handler WelcomeFunc) {
	registry.Lock()
	defer registry.Unlock()
	registry.welcome = handler
}

// RegisterErrorPage sets the HTTP 500 page used by App.Error. Owned by the auth area.
func RegisterErrorPage(handler ErrorFunc) {
	registry.Lock()
	defer registry.Unlock()
	registry.errPage = handler
}

// RegisterRoute adds a route under the context path. pattern is a net/http pattern relative to
// it, e.g. "GET /welcome.jsp" or "/register.jsp".
func RegisterRoute(pattern string, build RouteFunc) {
	registry.Lock()
	defer registry.Unlock()
	registry.routes = append(registry.routes, route{pattern: pattern, build: build})
}

// RegisterRestRoute adds a route under the REST root (/rest), e.g. "GET /addresses/{entryName}".
func RegisterRestRoute(pattern string, build RouteFunc) {
	registry.Lock()
	defer registry.Unlock()
	registry.routes = append(registry.routes, route{pattern: pattern, build: build, rest: true})
}

// RegisteredActions lists the registered action names (shown by the health endpoint).
func RegisteredActions() []string {
	registry.Lock()
	defer registry.Unlock()
	names := make([]string, 0, len(registry.actions))
	for name := range registry.actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
