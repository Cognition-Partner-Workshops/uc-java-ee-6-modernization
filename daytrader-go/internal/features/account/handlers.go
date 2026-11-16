package account

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

const recentOrderLimit = 5

type registerPageData struct {
	Results    string
	FullName   string
	Address    string
	Email      string
	UserID     string
	Money      string
	CreditCard string
}

type orderRow struct {
	Order *store.Order
	Total decimal.Decimal
}

type accountPageData struct {
	Page        web.Page
	Results     string
	Account     *store.Account
	Profile     *store.Profile
	TotalOrders int
	Orders      []orderRow
}

// renderRegisterPage is register.jsp: the form, prefilled from the request parameters.
func renderRegisterPage(app *web.App, w http.ResponseWriter, r *http.Request, results string) {
	data := registerPageData{
		Results:    results,
		FullName:   r.FormValue("Full Name"),
		Address:    r.FormValue("snail mail"),
		Email:      r.FormValue("email"),
		UserID:     r.FormValue("user id"),
		Money:      "10000",
		CreditCard: "123-fake-ccnum-456",
	}
	if r.Form.Has("money") {
		data.Money = r.FormValue("money")
	}
	if r.Form.Has("Credit Card Number") {
		data.CreditCard = r.FormValue("Credit Card Number")
	}
	app.Render(w, r, http.StatusOK, "account_register", data)
}

// register is TradeServletAction.doRegister.
func register(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	in := registerInput{
		userID:          r.FormValue("user id"),
		password:        r.FormValue("passwd"),
		confirmPassword: r.FormValue("confirm passwd"),
		fullName:        r.FormValue("Full Name"),
		address:         r.FormValue("snail mail"),
		email:           r.FormValue("email"),
		creditCard:      r.FormValue("Credit Card Number"),
		openBalance:     r.FormValue("money"),
	}
	fail := func(err error) {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doRegister(...) exception user =%s: %w", in.userID, err))
	}
	// The legacy servlet dereferences passwd before anything else: a missing one is a 500.
	if !r.Form.Has("passwd") {
		fail(errors.New("NullPointerException"))
		return
	}
	if !in.passwordsValid() {
		renderRegisterPage(app, w, r, "Registration operation failed, your passwords did not match")
		return
	}
	openBalance, err := parseOpenBalance(in.openBalance)
	if err != nil {
		fail(err)
		return
	}
	ctx := r.Context()
	accountID, err := app.Store.NextID(ctx, "account")
	if err != nil {
		fail(err)
		return
	}
	err = app.Store.InTx(ctx, func(tx pgx.Tx) error {
		return insertAccount(ctx, tx, accountID, in, openBalance, time.Now())
	})
	if errors.Is(err, errUserExists) {
		renderRegisterPage(app, w, r, "Registration operation failed;")
		return
	}
	if err != nil {
		fail(err)
		return
	}
	app.Forward(w, r, session, "login", map[string]string{"uid": in.userID, "passwd": in.password})
}

// account is TradeServletAction.doAccount. The status line comes from Attr "results".
func account(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	results, _ := web.Attr(r, "results")
	renderAccountPage(app, w, r, session, results)
}

func renderAccountPage(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session, results string) {
	ctx := r.Context()
	userID := session.UserID
	fail := func(err error) {
		app.Error(w, r, fmt.Errorf("TradeServletAction.doAccount(...) exception user =%s: %w", userID, err))
	}
	accountData, err := store.AccountByUser(ctx, app.Store.Pool, userID)
	if err != nil {
		fail(err)
		return
	}
	profile, err := store.ProfileByUser(ctx, app.Store.Pool, userID)
	if err != nil {
		fail(err)
		return
	}
	orders := accountOrders()
	data := accountPageData{
		Page:        app.NewPage(r, session, "DayTrader Account Information", "DayTrader Account"),
		Results:     results,
		Account:     accountData,
		Profile:     profile,
		TotalOrders: len(orders),
	}
	showAllOrders := r.Form.Has("showAllOrders")
	for index, order := range orders {
		if !showAllOrders && index >= recentOrderLimit {
			break
		}
		data.Orders = append(data.Orders, orderRow{Order: order, Total: order.Price.Mul(decimal.NewFromFloat(order.Quantity))})
	}
	app.Render(w, r, http.StatusOK, "account_account", data)
}

// updateProfile is TradeServletAction.doAccountUpdate: validate, update, then the account page.
func updateProfile(app *web.App, w http.ResponseWriter, r *http.Request, session *web.Session) {
	profile := &store.Profile{
		UserID:     session.UserID,
		Password:   javaTrim(r.FormValue("password")),
		FullName:   javaTrim(r.FormValue("fullname")),
		Address:    javaTrim(r.FormValue("address")),
		Email:      javaTrim(r.FormValue("email")),
		CreditCard: javaTrim(r.FormValue("creditcard")),
	}
	results := profileUpdateError(profile, javaTrim(r.FormValue("cpassword")))
	if results == "" {
		ctx := r.Context()
		err := app.Store.InTx(ctx, func(tx pgx.Tx) error { return saveProfile(ctx, tx, profile) })
		if err != nil {
			app.Error(w, r, fmt.Errorf("TradeServletAction.doAccountUpdate(...) exception user =%s: %w", session.UserID, err))
			return
		}
		results = "Account profile update successful"
	}
	renderAccountPage(app, w, r, session, results)
}
