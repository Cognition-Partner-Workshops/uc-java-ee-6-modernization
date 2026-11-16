package auth

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

type welcomeData struct {
	Results string
}

type errorData struct {
	RequestURI string
	StatusCode int
	Message    string
	Exception  string
}

// welcome is doWelcome + welcome.jsp: the login form with a status line.
func welcome(app *web.App, w http.ResponseWriter, r *http.Request, status string) {
	app.Render(w, r, http.StatusOK, "auth_welcome", welcomeData{Results: status})
}

// errorPage is error.jsp, the page behind web.xml's <error-page> for exceptions and HTTP 500.
// Unlike the legacy page it prints the error message only, never a stack trace.
func errorPage(app *web.App, w http.ResponseWriter, r *http.Request, err error) {
	app.Render(w, r, http.StatusInternalServerError, "auth_error", errorData{
		RequestURI: r.URL.Path,
		StatusCode: http.StatusInternalServerError,
		Message:    err.Error(),
		Exception:  rootCause(err).Error(),
	})
}

// rootCause unwraps to the innermost error (the legacy page shows the wrapped exception).
func rootCause(err error) error {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return err
		}
		err = inner
	}
}

// login is doLogin. A wrong password or an unknown user is an EJBException in the legacy bean,
// which the servlet rethrows as a ServletException: the answer is the HTTP 500 error page
// (parity/transcripts/09-invalid-login.json), not the welcome page.
func login(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	userID := web.Param(r, "uid")
	password := web.Param(r, "passwd")
	if _, err := loginAccount(r.Context(), app.Store, userID, password, time.Now().UTC().Truncate(time.Millisecond)); err != nil {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doLogin(...)Exception logging in user %s: %w", userID, err))
		return
	}
	session.UserID = userID
	session.CreationDate = time.Now()
	app.Forward(w, r, session, "home", map[string]string{"results": "Ready to Trade"})
}

// logout is doLogout: count the logout, invalidate the session, show the welcome page.
func logout(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	userID := session.UserID
	if err := logoutAccount(r.Context(), app.Store, userID); err != nil {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doLogout(...)exception logging out user %s: %w", userID, err))
		return
	}
	app.Sessions.Invalidate(w, session)
	welcome(app, w, r, "")
}
