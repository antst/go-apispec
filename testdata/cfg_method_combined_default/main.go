// Package main is the spec-009 guard for a `switch r.Method` whose responding arm is a
// COMBINED case (`case http.MethodGet, http.MethodPost:`) followed by a `default`.
// go/cfg lowers a combined case to ONE body block, so the dominator of that single block
// is the arm itself — not the switch tag. The dispatch root must therefore come from the
// recorded dispatch GROUP (every arm of the switch INCLUDING the default), not from the
// lone combined arm: only then is the default's 405 recognised as the dispatch fallback
// and NOT leaked onto the GET and POST operations the combined case fans out into.
package main

import (
	"encoding/json"
	"net/http"
)

// Item is the body returned for both GET and POST by the shared combined case.
type Item struct {
	ID int `json:"id"`
}

func items(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Item{ID: 1})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
