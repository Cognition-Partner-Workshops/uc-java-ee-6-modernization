// Package auth holds the auth feature area: login, logout, the welcome page and the HTTP 500
// page (legacy TradeServletAction.doLogin/doLogout/doWelcome, TradeSLSBBean.login/logout,
// welcome.jsp, error.jsp). Its handlers register themselves here from init(); see
// daytrader-go/README.md for the registry API. Only the auth workstream edits this package.
package auth

import (
	"net/http"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func init() {
	web.RegisterPublicAction("login", login)
	web.RegisterAction("logout", logout)
	web.RegisterWelcome(welcome)
	web.RegisterErrorPage(errorPage)
	web.RegisterRoute("GET /welcome.jsp", func(app *web.App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			welcome(app, w, r, "")
		})
	})
}
