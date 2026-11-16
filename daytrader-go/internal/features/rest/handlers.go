package rest

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"unicode/utf16"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

const (
	allowedMethods = "HEAD, GET, OPTIONS"
	wadlType       = "application/vnd.sun.wadl+xml"
	wadlHeader     = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><application xmlns="http://wadl.dev.java.net/2009/02">`
)

func utf16Units(text string) []uint16 {
	return utf16.Encode([]rune(text))
}

// listHandler is AddressBook.getAddresses: GET /addresses, @Produces application/json.
func listHandler(app *web.App, book *addressBook) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !resourceMethod(app, w, r) {
			return
		}
		writeAddressList(app, w, r, book.list(""))
	})
}

// searchHandler is AddressBook.search: entries whose name starts with {searchstring}.
func searchHandler(app *web.App, book *addressBook) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !resourceMethod(app, w, r) {
			return
		}
		writeAddressList(app, w, r, book.list(r.PathValue("searchstring")))
	})
}

// entryHandler is the sub-resource locator AddressBook.getAddress plus Address.get, with the
// behaviour recorded from Liberty (docs/BASELINE.md, parity/transcripts/13-rest-entry.json):
//
//   - unknown entry: 404 for every method (the locator returns null);
//   - Address.get is @Consumes text/xml or application/x-www-form-urlencoded, so a GET without
//     one of those Content-Types is 415. This is what a plain GET gets;
//   - it is @Produces text/xml, so an Accept header that excludes text/xml is 406;
//   - otherwise 204: the method compares List<MediaType> with a String, never builds a
//     response and returns null.
func entryHandler(app *web.App, book *addressBook) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entryName := r.PathValue("entryName")
		if _, found := book.get(entryName); !found {
			writeEmpty(w, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		case http.MethodOptions:
			writeWadl(w, wadlHeader+`<resources base="`+restBase(app, r)+`"><resource path="addresses/`+xmlAttr(entryName)+
				`"><method name="GET"><response><representation mediaType="text/xml"/></response></method></resource></resources></application>`)
			return
		default:
			methodNotAllowed(w)
			return
		}
		if !consumes(r, "text/xml", "application/x-www-form-urlencoded") {
			writeEmpty(w, http.StatusUnsupportedMediaType)
			return
		}
		if !accepts(r, "text/xml") {
			writeEmpty(w, http.StatusNotAcceptable)
			return
		}
		writeEmpty(w, http.StatusNoContent)
	})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeEmpty(w, http.StatusNotFound)
}

// resourceMethod answers everything but GET/HEAD for the two JSON resources and reports
// whether the caller should go on: OPTIONS gets the WADL, any other verb an empty 405.
func resourceMethod(app *web.App, w http.ResponseWriter, r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodOptions:
		writeWadl(w, wadlHeader+`<resources base="`+restBase(app, r)+`"><resource path="addresses">`+
			`<method name="GET"><response status="200"><representation mediaType="application/json"/></response></method>`+
			`<resource path="/search/{searchstring}"><param name="searchstring" style="template"/>`+
			`<method name="GET"><response status="200"><representation mediaType="application/json"/></response></method></resource>`+
			`<resource path="/{entryName}"><param name="entryName" style="template"/></resource></resource></resources></application>`)
	default:
		methodNotAllowed(w)
	}
	return false
}

func writeAddressList(app *web.App, w http.ResponseWriter, r *http.Request, addresses []address) {
	if !accepts(r, "application/json") {
		writeEmpty(w, http.StatusNotAcceptable)
		return
	}
	body, err := json.Marshal(addressList{Addresses: addresses})
	if err != nil {
		app.Error(w, r, err)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Content-Language", "en-US")
	header.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeWadl(w http.ResponseWriter, body string) {
	header := w.Header()
	header.Set("Allow", allowedMethods)
	header.Set("Content-Type", wadlType)
	header.Set("Content-Language", "en-US")
	header.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", allowedMethods)
	writeEmpty(w, http.StatusMethodNotAllowed)
}

// writeEmpty writes a status with no body and no Content-Type, as the legacy runtime does.
func writeEmpty(w http.ResponseWriter, status int) {
	header := w.Header()
	header.Set("Content-Language", "en-US")
	if status != http.StatusNoContent {
		header.Set("Content-Length", "0")
	}
	w.WriteHeader(status)
}

func restBase(app *web.App, r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	restPath := "/rest"
	if app != nil && app.Config != nil {
		restPath = app.Config.RestPath
	}
	return xmlAttr(scheme + "://" + r.Host + strings.TrimSuffix(restPath, "/") + "/")
}

func xmlAttr(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(value)
}

// consumes reports whether the request Content-Type is one of mediaTypes (@Consumes). A request
// without a Content-Type consumes nothing.
func consumes(r *http.Request, mediaTypes ...string) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	for _, mediaType := range mediaTypes {
		if contentType == mediaType {
			return true
		}
	}
	return false
}

// accepts reports whether the Accept header admits mediaType (@Produces). No Accept header
// admits everything; a range with q=0 admits nothing.
func accepts(r *http.Request, mediaType string) bool {
	values := r.Header.Values("Accept")
	if len(values) == 0 {
		return true
	}
	wantType, wantSubtype, _ := strings.Cut(mediaType, "/")
	for _, value := range values {
		for _, rangeText := range strings.Split(value, ",") {
			mediaRange, params, err := mime.ParseMediaType(strings.TrimSpace(rangeText))
			if err != nil {
				continue
			}
			if quality, hasQuality := params["q"]; hasQuality && strings.Trim(quality, "0.") == "" {
				continue
			}
			rangeType, rangeSubtype, _ := strings.Cut(mediaRange, "/")
			if (rangeType == "*" || rangeType == wantType) && (rangeSubtype == "*" || rangeSubtype == wantSubtype) {
				return true
			}
		}
	}
	return false
}
