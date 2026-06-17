// Package main exercises maps whose value type is a (same-package) struct, a
// slice of structs, and a pointer to a struct — the value type's package
// qualifier must attach to the element, not double the separator or wrap the
// slice (issue: map[string]Money → pkg..Money, map[string][]Money → pkg.._Money).
package main

import (
	"encoding/json"
	"net/http"
)

// Money is the map value struct.
type Money struct {
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
}

// Wallet holds several map-of-struct shapes.
type Wallet struct {
	Balances map[string]Money   `json:"balances"`
	History  map[string][]Money `json:"history"`
	Pending  map[string]*Money  `json:"pending"`
	Labels   map[string]string  `json:"labels"`
}

func getWallet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Wallet{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /wallet", getWallet)
	_ = http.ListenAndServe(":8080", mux)
}
