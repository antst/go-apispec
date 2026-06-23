// Package main exercises generic envelope substitution: a generic instantiation
// substitutes its concrete argument into every parameter-typed field, by name —
// including a two-parameter generic whose parameters are NOT named T/U/V.
package main

import (
	"encoding/json"
	"net/http"
)

// User is a concrete payload.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Order is a second concrete payload, for the second type parameter.
type Order struct {
	ID    int     `json:"id"`
	Total float64 `json:"total"`
}

// APIResponse is a single-parameter envelope (param T) — the already-working case,
// kept as a regression guard that positional and name-based binding agree for T.
type APIResponse[T any] struct {
	Data    T      `json:"data"`
	Message string `json:"message"`
}

// Pair is a two-parameter generic with NON-T/U/V parameter names. Positional
// T/U/V binding substitutes the wrong names; only name-based binding maps the
// first argument to K (First) and the second to V (Second).
type Pair[K any, V any] struct {
	First  K `json:"first"`
	Second V `json:"second"`
}

func getUser(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(APIResponse[User]{})
}

func getPair(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Pair[User, Order]{})
}

// getScores returns Pair instantiated with PRIMITIVE arguments — first/second
// resolve to string/int inline, and (unlike the former positional binding) no
// orphan "K-string"/"V-int" component is emitted.
func getScores(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Pair[string, int]{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", getUser)
	mux.HandleFunc("GET /pair", getPair)
	mux.HandleFunc("GET /scores", getScores)
	_ = http.ListenAndServe(":8080", mux)
}
