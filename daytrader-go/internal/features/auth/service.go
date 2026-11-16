package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// errLoginFailure is AccountDataBean.login's "Login failure" EJBException. The legacy message
// also carries the stored password; this one does not.
var errLoginFailure = errors.New("AccountBean:Login failure: incorrect password")

// errNoSuchUser is TradeSLSBBean.login's "No such user" EJBException.
var errNoSuchUser = errors.New("no such user")

// passwordMatches is profile.getPassword().equals(password): exact, case-sensitive.
func passwordMatches(stored *string, given string) bool {
	return stored != nil && *stored == given
}

// loginAccount is TradeSLSBBean.login + AccountDataBean.login in one transaction: check the
// password, then set lastlogin and add one to logincount. It returns the account id.
func loginAccount(ctx context.Context, db *store.Store, userID, password string, now time.Time) (int, error) {
	var accountID int
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		var stored *string
		err := tx.QueryRow(ctx, "SELECT passwd FROM accountprofileejb WHERE userid = $1", userID).Scan(&stored)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", errNoSuchUser, userID)
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, "SELECT accountid FROM accountejb WHERE profile_userid = $1 FOR UPDATE", userID).Scan(&accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s has no account", errNoSuchUser, userID)
		}
		if err != nil {
			return err
		}
		if !passwordMatches(stored, password) {
			return fmt.Errorf("%w for account %d (%s)", errLoginFailure, accountID, userID)
		}
		_, err = tx.Exec(ctx, "UPDATE accountejb SET lastlogin = $2, logincount = logincount + 1 WHERE accountid = $1", accountID, now)
		return err
	})
	return accountID, err
}

// logoutAccount is TradeSLSBBean.logout: add one to logoutcount. An unknown user is an error
// (the legacy bean fails with a NullPointerException, which is also an HTTP 500).
func logoutAccount(ctx context.Context, db *store.Store, userID string) error {
	return db.InTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE accountejb SET logoutcount = logoutcount + 1 WHERE profile_userid = $1", userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: %s", errNoSuchUser, userID)
		}
		return nil
	})
}
