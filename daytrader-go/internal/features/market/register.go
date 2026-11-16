// Package market holds the market feature area: the home page, the quotes page and the market
// summary. Its handlers register themselves here from init(); see daytrader-go/README.md for the
// registry API. Only the market workstream edits this package.
package market

import "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"

func init() {
	web.RegisterAction("home", home)
	web.RegisterAction("quotes", quotes)
}
