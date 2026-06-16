// Package main exercises chi middleware (router-level and group-level via Use)
// wrapping handlers, nested r.Route groups, and chi.URLParam path extraction —
// the middleware path the other chi fixtures don't cover.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Token is the resource.
type Token struct {
	Value string `json:"value"`
	Scope string `json:"scope"`
}

func issueToken(w http.ResponseWriter, r *http.Request) {
	var t Token
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(t)
}

func getToken(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	_ = json.NewEncoder(w).Encode(Token{})
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Route("/tokens", func(r chi.Router) {
		r.Use(middleware.NoCache)
		r.Post("/", issueToken)
		r.Get("/{id}", getToken)
	})
	_ = http.ListenAndServe(":8080", r)
}
