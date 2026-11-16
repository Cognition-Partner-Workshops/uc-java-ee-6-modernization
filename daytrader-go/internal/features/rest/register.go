// Package rest holds the rest feature area. Its handlers register themselves here from init();
// see daytrader-go/README.md for the registry API. Only the rest workstream edits this package.
package rest

import (
	"net/http"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

// The patterns carry no method: the legacy JAX-RS runtime answers other verbs with an empty
// 405 and "Allow: HEAD, GET, OPTIONS", which net/http's own method matching would not produce.
func init() {
	book := newAddressBook()
	web.RegisterRestRoute("/addresses", func(app *web.App) http.Handler { return listHandler(app, book) })
	web.RegisterRestRoute("/addresses/{$}", func(app *web.App) http.Handler { return listHandler(app, book) })
	web.RegisterRestRoute("/addresses/search/{searchstring}", func(app *web.App) http.Handler { return searchHandler(app, book) })
	web.RegisterRestRoute("/addresses/search/{searchstring}/{$}", func(app *web.App) http.Handler { return searchHandler(app, book) })
	web.RegisterRestRoute("/addresses/{entryName}", func(app *web.App) http.Handler { return entryHandler(app, book) })
	web.RegisterRestRoute("/addresses/{entryName}/{$}", func(app *web.App) http.Handler { return entryHandler(app, book) })
	web.RegisterRestRoute("/addresses/{unmatched...}", func(*web.App) http.Handler { return http.HandlerFunc(notFound) })
}
