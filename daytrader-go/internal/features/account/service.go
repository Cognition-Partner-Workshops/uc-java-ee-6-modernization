package account

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

var errUserExists = errors.New("account profile already exists")

// registerInput is the register form (TradeAppServlet reads these without trimming).
type registerInput struct {
	userID          string
	password        string
	confirmPassword string
	fullName        string
	address         string
	email           string
	creditCard      string
	openBalance     string
}

// passwordsValid is doRegister's check: both passwords equal and at least one character.
func (in registerInput) passwordsValid() bool {
	return in.password == in.confirmPassword && len(in.password) >= 1
}

// parseOpenBalance is new BigDecimal(openBalanceString): no surrounding whitespace, no empty string.
func parseOpenBalance(text string) (decimal.Decimal, error) {
	if text == "" || strings.TrimSpace(text) != text {
		return decimal.Decimal{}, errors.New("NumberFormatException: " + text)
	}
	return decimal.NewFromString(text)
}

// insertAccount is TradeSLSBBean.register: errUserExists when the profile is already there,
// otherwise one profile row and one account row. The legacy bean leaves lastLogin null until
// the login that doRegister runs straight after; here it starts at the creation date because
// store.Account.LastLogin is not nullable.
func insertAccount(ctx context.Context, tx pgx.Tx, accountID int, in registerInput, openBalance decimal.Decimal, now time.Time) error {
	var existing int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM accountprofileejb WHERE userid = $1", in.userID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return errUserExists
	}
	if _, err := tx.Exec(ctx, "INSERT INTO accountprofileejb (userid, passwd, fullname, address, email, creditcard) VALUES ($1, $2, $3, $4, $5, $6)",
		in.userID, in.password, in.fullName, in.address, in.email, in.creditCard); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO accountejb (accountid, logincount, logoutcount, lastlogin, creationdate, balance, openbalance, profile_userid) "+
		"VALUES ($1, 0, 0, $2, $2, $3, $3, $4)", accountID, now, openBalance, in.userID)
	return err
}

// profileUpdateError is doAccountUpdate's input check; "" means the update may run.
func profileUpdateError(profile *store.Profile, confirmPassword string) string {
	if profile.Password != confirmPassword {
		return "Update profile error: passwords do not match"
	}
	if profile.Password == "" || profile.FullName == "" || profile.Address == "" || profile.CreditCard == "" || profile.Email == "" {
		return "Update profile error: please fill in all profile information fields"
	}
	return ""
}

// saveProfile is TradeSLSBBean.updateAccountProfile.
func saveProfile(ctx context.Context, tx pgx.Tx, profile *store.Profile) error {
	tag, err := tx.Exec(ctx, "UPDATE accountprofileejb SET address = $2, passwd = $3, fullname = $4, creditcard = $5, email = $6 WHERE userid = $1",
		profile.UserID, profile.Address, profile.Password, profile.FullName, profile.CreditCard, profile.Email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// accountOrders is what account.jsp gets as orderDataBeans. The legacy page always receives an
// empty collection in the default Full EJB3 mode (docs/BASELINE.md quirk 2), so it prints
// "Total Orders: 0" and an empty recent-orders table whatever the user has traded. Kept as recorded.
func accountOrders() []*store.Order {
	return []*store.Order{}
}

// javaTrim is String.trim(): strips leading and trailing characters up to U+0020.
func javaTrim(text string) string {
	return strings.TrimFunc(text, func(r rune) bool { return r <= ' ' })
}
