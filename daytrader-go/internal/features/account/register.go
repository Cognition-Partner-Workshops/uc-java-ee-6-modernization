// Package account holds the account feature area. Its handlers register themselves here from init();
// see daytrader-go/README.md for the registry API. Only the account workstream edits this package.
package account

import (
	"net/http"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func init() {
	web.RegisterPublicAction("register", register)
	web.RegisterRoute("GET /register.jsp", func(app *web.App) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			renderRegisterPage(app, w, r, "")
		})
	})
	web.RegisterAction("account", account)
	web.RegisterAction("update_profile", updateProfile)
}
