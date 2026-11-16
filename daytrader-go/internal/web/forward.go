package web

import (
	"context"
	"net/http"
)

type attrsKey struct{}

// Forward runs another registered action in the same request, the way the legacy servlet
// chains doRegister -> doLogin -> doHome. attrs are request attributes for the target handler:
// read them with Attr, or with Param when they override a form parameter. This is how one
// feature area hands over to another without importing it.
//
// Contracts between areas:
//   - "home" (market): Attr "results" is the status line; default "Ready to Trade".
//   - "login" (auth): Param "uid" and Param "passwd", so register can log the new user in.
//   - "account" (account), "portfolio" (trading), "quotes" (market): Attr "results" is the status line.
func (a *App) Forward(w http.ResponseWriter, r *http.Request, session *Session, actionName string, attrs map[string]string) {
	registry.Lock()
	entry, found := registry.actions[actionName]
	registry.Unlock()
	if !found {
		a.Welcome(w, r, "TradeAppServlet: Invalid Action"+actionName)
		return
	}
	merged := map[string]string{}
	if existing, ok := r.Context().Value(attrsKey{}).(map[string]string); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	for key, value := range attrs {
		merged[key] = value
	}
	entry.handler(a, w, r.WithContext(context.WithValue(r.Context(), attrsKey{}, merged)), session)
}

// Attr returns a request attribute set by Forward.
func Attr(r *http.Request, name string) (string, bool) {
	attrs, _ := r.Context().Value(attrsKey{}).(map[string]string)
	value, ok := attrs[name]
	return value, ok
}

// Param returns the request attribute when Forward set one, otherwise the form parameter.
func Param(r *http.Request, name string) string {
	if value, ok := Attr(r, name); ok {
		return value
	}
	return r.FormValue(name)
}
