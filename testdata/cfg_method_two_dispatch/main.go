// Package main is the spec-009 guard for a handler with TWO separate `switch r.Method`
// dispatches and an INDEPENDENT conditional between them. The recorded method arms span
// both dispatches; the classifier must scope the dispatch root to the arms of ONE
// dispatch (the responding one) so the independent 401 between the dispatches is shared
// onto every method — not over-excluded by a root inflated to cover both dispatches.
package main

import (
	"encoding/json"
	"net/http"
)

// GetR is the GET response body.
type GetR struct {
	A int `json:"a"`
}

// PostR is the POST response body.
type PostR struct {
	B int `json:"b"`
}

// AuthErr is the independent 401 body (reachable on every method).
type AuthErr struct {
	M string `json:"m"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method { // dispatch 1: per-method headers
	case http.MethodGet:
		w.Header().Set("X-A", "1")
	case http.MethodPost:
		w.Header().Set("X-B", "1")
	}
	if r.Header.Get("X-Auth") == "" { // independent of method, between the dispatches
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(AuthErr{M: "no auth"})
		return
	}
	switch r.Method { // dispatch 2: per-method bodies
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(GetR{A: 1})
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PostR{B: 2})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", handler)
	_ = http.ListenAndServe(":8080", mux)
}
