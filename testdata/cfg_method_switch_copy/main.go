// Package main is the spec-009 guard for a `switch` over a COPY of r.Method
// (`m := r.Method; switch m { … default }`). The switch TAG is `m`, not the bare
// `r.Method` selector, so the dispatch is recognised by its method-named CASE VALUES — the
// same test the route splitter uses to attribute a case to a method — rather than by the
// tag. The `default` 405 is then a recognised dispatch fallback and excluded, not leaked
// onto the GET and POST operations. Tag-gated detection regressed this common idiom.
package main

import (
	"encoding/json"
	"net/http"
)

// GetItem is the GET response body.
type GetItem struct {
	ID int `json:"id"`
}

// PostItem is the POST response body.
type PostItem struct {
	Name string `json:"name"`
}

func items(w http.ResponseWriter, r *http.Request) {
	m := r.Method
	switch m {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(GetItem{ID: 1})
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PostItem{Name: "x"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
