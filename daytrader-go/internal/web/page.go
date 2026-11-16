package web

import (
	"net/http"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
)

// Page is what the shared layout blocks ("pageStart", "pageEnd") need. Embed it in a page's
// data struct as the field Page.
type Page struct {
	Title        string // <TITLE>, e.g. "Welcome to DayTrader"
	Banner       string // red bar text, e.g. "DayTrader Home"
	ClosedOrders []*store.Order
}

// NewPage builds the layout data for a signed-in page and collects the user's closed orders
// (which marks them completed, as every legacy JSP does through getClosedOrders).
func (a *App) NewPage(r *http.Request, session *Session, title, banner string) Page {
	page := Page{Title: title, Banner: banner}
	if session != nil && session.UserID != "" {
		page.ClosedOrders = a.ClosedOrders(r.Context(), session.UserID)
	}
	return page
}
