// Package main is the spec-009 fixture for method-conditional dispatch via a
// `switch r.Method` whose cases are net/http METHOD CONSTANTS (`case http.MethodGet:`).
// It mirrors cfg_method_if_dispatch (the `if r.Method ==` form): the analyzer must
// split into one operation per method identically — whether the dispatch is a switch
// or an if, and whether the cases are string literals or net/http constants (FR-003).
package main

import (
	"encoding/json"
	"net/http"
)

// CreatedItem is the POST response body.
type CreatedItem struct {
	ID int `json:"id"`
}

// ItemList is the GET response body.
type ItemList struct {
	Items []string `json:"items"`
}

func items(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreatedItem{ID: 1})
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ItemList{Items: []string{"a"}})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
