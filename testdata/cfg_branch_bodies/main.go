// Package main is the spec-009 fixture for branch-dependent response bodies
// (US3, FR-005): a handler that writes FullUser with 200 on one branch and
// ErrorBody with 404 on another. Each status must carry the body actually
// written on its path, not a single shape merged across branches.
package main

import (
	"encoding/json"
	"net/http"
)

// FullUser is the 200 body.
type FullUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ErrorBody is the 404 body.
type ErrorBody struct {
	Message string `json:"message"`
}

func getUser(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorBody{Message: "missing id"})
	} else {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(FullUser{ID: 1, Name: "ada"})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", getUser)
	_ = http.ListenAndServe(":8080", mux)
}
