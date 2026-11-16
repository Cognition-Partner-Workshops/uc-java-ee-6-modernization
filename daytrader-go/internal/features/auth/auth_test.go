package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func newApp(t *testing.T, db *store.Store) *web.App {
	t.Helper()
	cfg, err := config.FromEnv(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	app, err := web.New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// testStore connects to the PostgreSQL from `make go-db`; tests that need it are skipped without it.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://daytrader:daytrader@localhost:55432/daytrader?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := store.Open(ctx, databaseURL, 4, 1000)
	if err != nil {
		t.Skipf("PostgreSQL not available (%v); run make go-seed", err)
	}
	var table *string
	if err := db.Pool.QueryRow(ctx, "SELECT to_regclass('accountejb')::text").Scan(&table); err != nil || table == nil {
		db.Close()
		t.Skip("schema not created; run make go-seed")
	}
	t.Cleanup(db.Close)
	return db
}

// addUser inserts a throwaway profile + account and removes them when the test ends.
func addUser(t *testing.T, db *store.Store, password string) (string, int) {
	t.Helper()
	ctx := context.Background()
	accountID := -int(time.Now().UnixNano() % 1_000_000_000)
	userID := fmt.Sprintf("authtest:%d", accountID)
	if _, err := db.Pool.Exec(ctx, "INSERT INTO accountprofileejb (userid, passwd, fullname) VALUES ($1, $2, 'Auth Test')", userID, password); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO accountejb (accountid, logincount, logoutcount, creationdate, balance, openbalance, profile_userid) "+
		"VALUES ($1, 0, 0, now(), 100, 100, $2)", accountID, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DELETE FROM accountejb WHERE accountid = $1", accountID)
		_, _ = db.Pool.Exec(ctx, "DELETE FROM accountprofileejb WHERE userid = $1", userID)
	})
	return userID, accountID
}

func counts(t *testing.T, db *store.Store, accountID int) (logins, logouts int, lastLogin *time.Time) {
	t.Helper()
	err := db.Pool.QueryRow(context.Background(), "SELECT logincount, logoutcount, lastlogin FROM accountejb WHERE accountid = $1", accountID).
		Scan(&logins, &logouts, &lastLogin)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func get(t *testing.T, client *http.Client, target string) (int, string, string) {
	t.Helper()
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	return read(t, response)
}

func read(t *testing.T, response *http.Response) (int, string, string) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header.Get("Content-Type"), string(body)
}

func TestPasswordMatches(t *testing.T) {
	stored := "xxx"
	cases := []struct {
		stored *string
		given  string
		want   bool
	}{
		{&stored, "xxx", true},
		{&stored, "XXX", false},
		{&stored, "xxx ", false},
		{&stored, "", false},
		{nil, "", false},
		{nil, "xxx", false},
	}
	for _, c := range cases {
		if got := passwordMatches(c.stored, c.given); got != c.want {
			t.Errorf("passwordMatches(%v, %q) = %v, want %v", c.stored, c.given, got, c.want)
		}
	}
}

func TestRootCause(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", errLoginFailure))
	if got := rootCause(wrapped); got != errLoginFailure {
		t.Errorf("rootCause = %v", got)
	}
}

func TestWelcomePages(t *testing.T) {
	server := httptest.NewServer(newApp(t, nil).Handler())
	defer server.Close()
	base := server.URL + "/daytrader"

	for _, path := range []string{"/app", "/welcome.jsp"} {
		status, contentType, body := get(t, server.Client(), base+path)
		if status != http.StatusOK || contentType != "text/html" {
			t.Errorf("%s: status %d, content type %q", path, status, contentType)
		}
		for _, want := range []string{
			"<TITLE>DayTrader Login</TITLE>", `<FORM action="app" method="POST">`, `name="uid" value="uid:0"`,
			`name="passwd" value="xxx"`, `name="action"`, `value="login"`, `<A href="register.jsp">Register&nbsp;With&nbsp;DayTrader</A>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: missing %q", path, want)
			}
		}
		if strings.Contains(body, "User Not Logged in") {
			t.Errorf("%s: unexpected status line", path)
		}
	}

	// A protected action without a login shows the welcome page with the legacy status line.
	for _, action := range []string{"logout", "home"} {
		status, _, body := get(t, server.Client(), base+"/app?action="+action)
		if status != http.StatusOK || !strings.Contains(body, "User Not Logged in") || !strings.Contains(body, "DayTrader Login") {
			t.Errorf("action=%s without login: status %d", action, status)
		}
	}
}

func TestWelcomeEscapesStatus(t *testing.T) {
	app := newApp(t, nil)
	recorder := httptest.NewRecorder()
	welcome(app, recorder, httptest.NewRequest(http.MethodGet, "/daytrader/app", nil), "<script>alert(1)</script>")
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Error("status line is not escaped")
	}
}

func TestErrorPage(t *testing.T) {
	app := newApp(t, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/daytrader/app", nil)
	app.Error(recorder, request, fmt.Errorf("doLogin failed: %w", errLoginFailure))
	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError || recorder.Header().Get("Content-Type") != "text/html" {
		t.Errorf("status %d, content type %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		"DayTrader Error", "An Error has occured during DayTrader processing", "<b>Processing request:</b>/daytrader/app",
		"<b>StatusCode:</b> 500", "<b>Message:</b>doLogin failed", "<b>Exception:</b>" + errLoginFailure.Error(),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestLoginAccount(t *testing.T) {
	db := testStore(t)
	ctx := context.Background()
	userID, accountID := addUser(t, db, "secret")
	now := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

	if _, err := loginAccount(ctx, db, userID, "wrong", now); !errors.Is(err, errLoginFailure) {
		t.Fatalf("wrong password: %v", err)
	} else if strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks the stored password: %v", err)
	}
	if _, err := loginAccount(ctx, db, userID+"-nobody", "secret", now); !errors.Is(err, errNoSuchUser) {
		t.Fatalf("unknown user: %v", err)
	}
	if logins, _, lastLogin := counts(t, db, accountID); logins != 0 || lastLogin != nil {
		t.Fatalf("failed logins changed the account: logincount %d, lastlogin %v", logins, lastLogin)
	}

	gotID, err := loginAccount(ctx, db, userID, "secret", now)
	if err != nil || gotID != accountID {
		t.Fatalf("login: id %d, err %v", gotID, err)
	}
	if _, err := loginAccount(ctx, db, userID, "secret", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	logins, logouts, lastLogin := counts(t, db, accountID)
	if logins != 2 || logouts != 0 || lastLogin == nil || !lastLogin.Equal(now.Add(time.Hour)) {
		t.Errorf("logincount %d, logoutcount %d, lastlogin %v", logins, logouts, lastLogin)
	}

	if err := logoutAccount(ctx, db, userID); err != nil {
		t.Fatal(err)
	}
	if _, logouts, _ := counts(t, db, accountID); logouts != 1 {
		t.Errorf("logoutcount %d", logouts)
	}
	if err := logoutAccount(ctx, db, userID+"-nobody"); !errors.Is(err, errNoSuchUser) {
		t.Errorf("logout of unknown user: %v", err)
	}
}

func TestLoginLogoutOverHTTP(t *testing.T) {
	db := testStore(t)
	userID, accountID := addUser(t, db, "secret")
	server := httptest.NewServer(newApp(t, db).Handler())
	defer server.Close()
	base := server.URL + "/daytrader"
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	post := func(password string) (int, string) {
		response, err := client.PostForm(base+"/app", url.Values{"action": {"login"}, "uid": {userID}, "passwd": {password}})
		if err != nil {
			t.Fatal(err)
		}
		status, _, body := read(t, response)
		return status, body
	}

	status, body := post("wrong")
	if status != http.StatusInternalServerError || !strings.Contains(body, "An Error has occured during DayTrader processing") {
		t.Fatalf("invalid login: status %d", status)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "wrong") {
		t.Error("error page shows a password")
	}
	if status, _, body := get(t, client, base+"/app?action=logout"); status != http.StatusOK || !strings.Contains(body, "User Not Logged in") {
		t.Errorf("an invalid login must not sign the session in (status %d)", status)
	}

	// The login hands over to "home"; with no home registered the scaffold answers 200.
	if status, _ := post("secret"); status != http.StatusOK {
		t.Fatalf("login: status %d", status)
	}
	if logins, _, _ := counts(t, db, accountID); logins != 1 {
		t.Errorf("logincount %d", logins)
	}

	status, _, body = get(t, client, base+"/app?action=logout")
	if status != http.StatusOK || !strings.Contains(body, "DayTrader Login") || strings.Contains(body, "User Not Logged in") {
		t.Errorf("logout: status %d", status)
	}
	if _, logouts, _ := counts(t, db, accountID); logouts != 1 {
		t.Errorf("logoutcount %d", logouts)
	}
	if _, _, body := get(t, client, base+"/app?action=logout"); !strings.Contains(body, "User Not Logged in") {
		t.Error("the session survived the logout")
	}
}
