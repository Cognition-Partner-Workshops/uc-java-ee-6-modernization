package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestMux wires the handlers the way register.go does, under /rest.
func newTestMux() *http.ServeMux {
	book := newAddressBook()
	mux := http.NewServeMux()
	mux.Handle("/rest/addresses", listHandler(nil, book))
	mux.Handle("/rest/addresses/{$}", listHandler(nil, book))
	mux.Handle("/rest/addresses/search/{searchstring}", searchHandler(nil, book))
	mux.Handle("/rest/addresses/search/{searchstring}/{$}", searchHandler(nil, book))
	mux.Handle("/rest/addresses/{entryName}", entryHandler(nil, book))
	mux.Handle("/rest/addresses/{entryName}/{$}", entryHandler(nil, book))
	mux.Handle("/rest/addresses/{unmatched...}", http.HandlerFunc(notFound))
	return mux
}

func call(t *testing.T, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://localhost:8080"+path, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	newTestMux().ServeHTTP(recorder, request)
	return recorder
}

const entry1JSON = `{"entryName":"Entry1","zipCode":"12345","streetAddress":"Address","city":"City","state":"State","country":"Country"}`

func TestListMatchesLegacyBody(t *testing.T) {
	for _, path := range []string{"/rest/addresses", "/rest/addresses/"} {
		response := call(t, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, response.Code)
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Errorf("%s: content type %q", path, contentType)
		}
		var list addressList
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range list.Addresses {
			names = append(names, entry.EntryName)
		}
		// The order recorded from Liberty (java.util.HashMap iteration order).
		if got := strings.Join(names, ","); got != "Entry4,Entry5,Entry2,Entry3,Entry1" {
			t.Errorf("%s: order %s", path, got)
		}
		if response.Body.Len() != 600 {
			t.Errorf("%s: body length %d, legacy is 600", path, response.Body.Len())
		}
		if !strings.HasSuffix(response.Body.String(), entry1JSON+"]}") {
			t.Errorf("%s: unexpected body %s", path, response.Body.String())
		}
	}
}

func TestSearch(t *testing.T) {
	cases := map[string]string{
		"/rest/addresses/search/Entry1":   `{"addresses":[` + entry1JSON + `]}`,
		"/rest/addresses/search/Entry1/":  `{"addresses":[` + entry1JSON + `]}`,
		"/rest/addresses/search/E%6Etry1": `{"addresses":[` + entry1JSON + `]}`,
		"/rest/addresses/search/Zed":      `{"addresses":[]}`,
		"/rest/addresses/search/entry1":   `{"addresses":[]}`,
	}
	for path, want := range cases {
		response := call(t, http.MethodGet, path, nil)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Errorf("%s: %d %s", path, response.Code, response.Body.String())
		}
	}
	response := call(t, http.MethodGet, "/rest/addresses/search/E", nil)
	var list addressList
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil || len(list.Addresses) != 5 {
		t.Errorf("prefix E: %v, %d entries", err, len(list.Addresses))
	}
}

func TestStatuses(t *testing.T) {
	xml := map[string]string{"Content-Type": "text/xml"}
	cases := []struct {
		method  string
		path    string
		headers map[string]string
		status  int
	}{
		// parity/transcripts/13-rest-entry.json
		{http.MethodGet, "/rest/addresses/Entry2", nil, http.StatusUnsupportedMediaType},
		{http.MethodGet, "/rest/addresses/Nope", nil, http.StatusNotFound},
		// the rest was observed on the running Liberty baseline
		{http.MethodGet, "/rest/addresses/Entry2/", nil, http.StatusUnsupportedMediaType},
		{http.MethodHead, "/rest/addresses/Entry2", nil, http.StatusUnsupportedMediaType},
		{http.MethodGet, "/rest/addresses/Entry2", map[string]string{"Accept": "text/xml"}, http.StatusUnsupportedMediaType},
		{http.MethodGet, "/rest/addresses/Entry2", map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{http.MethodGet, "/rest/addresses/Entry2", xml, http.StatusNoContent},
		{http.MethodHead, "/rest/addresses/Entry2", xml, http.StatusNoContent},
		{http.MethodGet, "/rest/addresses/Entry2", map[string]string{"Content-Type": "text/xml", "Accept": "text/xml"}, http.StatusNoContent},
		{http.MethodGet, "/rest/addresses/Entry2", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusNoContent},
		{http.MethodGet, "/rest/addresses/Entry2", map[string]string{"Content-Type": "text/xml; charset=utf-8", "Accept": "application/json"}, http.StatusNotAcceptable},
		{http.MethodGet, "/rest/addresses/Nope", xml, http.StatusNotFound},
		{http.MethodGet, "/rest/addresses/Entry2/x", nil, http.StatusNotFound},
		{http.MethodPost, "/rest/addresses/zz/y", nil, http.StatusNotFound},
		{http.MethodGet, "/rest/addresses/search", nil, http.StatusNotFound},
		{http.MethodGet, "/rest/addresses/search/", nil, http.StatusNotFound},
		{http.MethodDelete, "/rest/addresses/Nope", nil, http.StatusNotFound},
		{http.MethodOptions, "/rest/addresses/Nope", nil, http.StatusNotFound},
		{http.MethodPost, "/rest/addresses", nil, http.StatusMethodNotAllowed},
		{http.MethodPut, "/rest/addresses/Entry2", nil, http.StatusMethodNotAllowed},
		{http.MethodPost, "/rest/addresses/Entry2", xml, http.StatusMethodNotAllowed},
		{http.MethodDelete, "/rest/addresses/Entry2", nil, http.StatusMethodNotAllowed},
		{http.MethodDelete, "/rest/addresses/search/Entry1", nil, http.StatusMethodNotAllowed},
		{http.MethodGet, "/rest/addresses", map[string]string{"Accept": "text/xml"}, http.StatusNotAcceptable},
		{http.MethodGet, "/rest/addresses/search/E", map[string]string{"Accept": "application/xml"}, http.StatusNotAcceptable},
		{http.MethodGet, "/rest/addresses", map[string]string{"Accept": "application/*"}, http.StatusOK},
		{http.MethodGet, "/rest/addresses", map[string]string{"Accept": "text/html, */*;q=0.1"}, http.StatusOK},
		{http.MethodHead, "/rest/addresses", nil, http.StatusOK},
	}
	for _, test := range cases {
		response := call(t, test.method, test.path, test.headers)
		if response.Code != test.status {
			t.Errorf("%s %s %v: status %d, want %d", test.method, test.path, test.headers, response.Code, test.status)
			continue
		}
		if test.status == http.StatusOK {
			continue
		}
		if response.Body.Len() != 0 {
			t.Errorf("%s %s: body %q, want empty", test.method, test.path, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "" {
			t.Errorf("%s %s: content type %q, want none", test.method, test.path, contentType)
		}
		if allow := response.Header().Get("Allow"); test.status == http.StatusMethodNotAllowed && allow != "HEAD, GET, OPTIONS" {
			t.Errorf("%s %s: Allow %q", test.method, test.path, allow)
		}
	}
}

func TestOptionsWadl(t *testing.T) {
	collection := call(t, http.MethodOptions, "/rest/addresses", nil)
	search := call(t, http.MethodOptions, "/rest/addresses/search/Entry1", nil)
	entry := call(t, http.MethodOptions, "/rest/addresses/Entry2", nil)
	for name, response := range map[string]*httptest.ResponseRecorder{"collection": collection, "search": search, "entry": entry} {
		if response.Code != http.StatusOK || response.Header().Get("Allow") != "HEAD, GET, OPTIONS" ||
			response.Header().Get("Content-Type") != "application/vnd.sun.wadl+xml" {
			t.Errorf("%s: %d %v", name, response.Code, response.Header())
		}
		if !strings.Contains(response.Body.String(), `<resources base="http://localhost:8080/rest/">`) {
			t.Errorf("%s: %s", name, response.Body.String())
		}
	}
	// Body lengths recorded from Liberty on localhost:9083 (same host length as localhost:8080).
	if collection.Body.Len() != 614 || search.Body.Len() != 614 || entry.Body.Len() != 313 {
		t.Errorf("wadl lengths %d %d %d, legacy 614 614 313", collection.Body.Len(), search.Body.Len(), entry.Body.Len())
	}
}

func TestJavaHashMapOrder(t *testing.T) {
	if bucket := javaHashMapBucket("Entry1", 16); bucket != javaHashMapBucket("Entry1", 16) || bucket > 15 {
		t.Fatalf("bucket %d", bucket)
	}
	if capacity := javaHashMapCapacity(13); capacity != 32 {
		t.Errorf("capacity for 13 entries %d, want 32", capacity)
	}
	book := newAddressBook()
	book.store(address{EntryName: "Entry2", City: "Changed"})
	if entry, found := book.get("Entry2"); !found || entry.City != "Changed" || len(book.list("")) != 5 {
		t.Errorf("store did not replace Entry2: %+v", entry)
	}
}
