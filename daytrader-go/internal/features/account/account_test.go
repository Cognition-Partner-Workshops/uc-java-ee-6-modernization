package account

import (
	"bytes"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	daytradergo "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func newTestApp(t *testing.T) *web.App {
	t.Helper()
	app, err := web.New(&config.Config{ContextPath: "/daytrader", RestPath: "/rest"}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return app
}

func renderTemplate(t *testing.T, name string, data any) string {
	t.Helper()
	templates, err := template.New("daytrader").Funcs(web.TemplateFuncs).ParseFS(daytradergo.Templates, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	var buffer bytes.Buffer
	if err := templates.ExecuteTemplate(&buffer, name, data); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return buffer.String()
}

func TestPasswordsValid(t *testing.T) {
	cases := []struct {
		password, confirm string
		want              bool
	}{
		{"pw", "pw", true},
		{"pw", "other", false},
		{"", "", false},
		{" ", " ", true},
	}
	for _, c := range cases {
		got := registerInput{password: c.password, confirmPassword: c.confirm}.passwordsValid()
		if got != c.want {
			t.Errorf("passwordsValid(%q, %q) = %v, want %v", c.password, c.confirm, got, c.want)
		}
	}
}

func TestParseOpenBalance(t *testing.T) {
	balance, err := parseOpenBalance("100000")
	if err != nil || !balance.Equal(decimal.NewFromInt(100000)) {
		t.Errorf("parseOpenBalance(100000) = %v, %v", balance, err)
	}
	for _, text := range []string{"", " 100", "100 ", "abc"} {
		if _, err := parseOpenBalance(text); err == nil {
			t.Errorf("parseOpenBalance(%q) should fail like new BigDecimal", text)
		}
	}
}

func TestProfileUpdateError(t *testing.T) {
	full := store.Profile{UserID: "uid:1", Password: "pw", FullName: "n", Address: "a", Email: "e", CreditCard: "c"}
	if got := profileUpdateError(&full, "pw"); got != "" {
		t.Errorf("valid profile: got %q", got)
	}
	if got := profileUpdateError(&full, "other"); got != "Update profile error: passwords do not match" {
		t.Errorf("mismatch: got %q", got)
	}
	missing := full
	missing.Email = ""
	if got := profileUpdateError(&missing, "pw"); got != "Update profile error: please fill in all profile information fields" {
		t.Errorf("missing field: got %q", got)
	}
}

func TestJavaTrim(t *testing.T) {
	if got := javaTrim(" \t2 New Road\n "); got != "2 New Road" {
		t.Errorf("javaTrim = %q", got)
	}
}

func TestAccountOrdersQuirk(t *testing.T) {
	if got := len(accountOrders()); got != 0 {
		t.Errorf("account page must list no orders (BASELINE quirk 2), got %d", got)
	}
}

func TestAccountTemplate(t *testing.T) {
	created := time.Date(2026, 11, 4, 7, 42, 47, 0, time.UTC)
	html := renderTemplate(t, "account_account", accountPageData{
		Page:    web.Page{Title: "DayTrader Account Information", Banner: "DayTrader Account"},
		Results: "Account profile update successful",
		Account: &store.Account{AccountID: 15001, LoginCount: 1, LastLogin: created, CreationDate: created,
			Balance: decimal.RequireFromString("99975.05"), OpenBalance: decimal.NewFromInt(100000), ProfileID: "ru:1"},
		Profile: &store.Profile{UserID: "ru:1", Password: "pw2", FullName: "Renamed <Tester>", Address: "2 New Road",
			Email: "renamed@example.com", CreditCard: "1111-2222-3333-4444"},
		TotalOrders: 0,
	})
	for _, want := range []string{
		"<TITLE>DayTrader Account Information</TITLE>",
		`<FONT color="#ff0000">Account profile update successful</FONT>`,
		"Wed Nov 04 07:42:47 UTC 2026",
		"<TD valign=\"bottom\">99975.05</TD>",
		"<TD valign=\"bottom\">100000.00</TD>",
		"<B>Total Orders: </B>0</TD>",
		"<B>Recent Orders</B>",
		`name="userID"
							value="ru:1"`,
		`value="Renamed &lt;Tester&gt;"`,
		`name="cpassword"
							value="pw2"`,
		`value="update_profile"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("account page is missing %q", want)
		}
	}
	if strings.Contains(html, `bgcolor="#fafcb6"`) {
		t.Error("account page must not print order rows")
	}
}

func TestRegisterPageDefaults(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/daytrader/register.jsp", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/html" {
		t.Fatalf("status %d, content type %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		"<TITLE>DayTrader Registration</TITLE>",
		`name="money" value='10000'`,
		`value="123-fake-ccnum-456" readonly`,
		`name="user id" value=""`,
		`<INPUT type="hidden" name="action" value="register">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("register page is missing %q", want)
		}
	}
}

func postRegister(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/daytrader/app", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	newTestApp(t).Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestRegisterPasswordMismatch(t *testing.T) {
	recorder := postRegister(t, url.Values{
		"action": {"register"}, "user id": {"ru:9"}, "passwd": {"pw"}, "confirm passwd": {"other"},
		"Full Name": {"Parity Tester"}, "Credit Card Number": {"1111"}, "money": {"500"}, "email": {"p@example.com"},
		"snail mail": {"1 Test Street"},
	})
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d", recorder.Code)
	}
	for _, want := range []string{
		"Registration operation failed, your passwords did not match",
		`name="Full Name"
				value="Parity Tester"`,
		`name="user id" value="ru:9"`,
		`name="money" value='500'`,
		`value="1111" readonly`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("register page is missing %q", want)
		}
	}
}

func TestRegisterBadMoneyIsServerError(t *testing.T) {
	recorder := postRegister(t, url.Values{
		"action": {"register"}, "user id": {"ru:9"}, "passwd": {"pw"}, "confirm passwd": {"pw"}, "money": {"lots"},
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", recorder.Code)
	}
}

func TestRegisterMissingPasswordIsServerError(t *testing.T) {
	recorder := postRegister(t, url.Values{"action": {"register"}, "user id": {"ru:9"}})
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", recorder.Code)
	}
}

func TestAccountRequiresLogin(t *testing.T) {
	for _, name := range []string{"account", "update_profile"} {
		recorder := httptest.NewRecorder()
		newTestApp(t).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/daytrader/app?action="+name, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "User Not Logged in") {
			t.Errorf("%s without a session: status %d, body lacks the not-logged-in line", name, recorder.Code)
		}
	}
}
