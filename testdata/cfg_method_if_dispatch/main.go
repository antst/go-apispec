// Package main is the spec-009 fixture for method-conditional dispatch via
// `if r.Method == …` guards (US2, FR-003). The handler is registered without a
// method, so each guarded arm is reachable: the POST arm creates an item (201)
// and the GET arm lists items (200). The analyzer must split this into one
// operation per reachable method, exactly as it does for a `switch r.Method`.
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
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreatedItem{ID: 1})
	} else if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ItemList{Items: []string{"a"}})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
