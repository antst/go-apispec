// Package main exercises the required/optional matrix: json omitempty (optional),
// no omitempty (required by Go default), validate:"required" (required), and a
// mix on a single DTO — exercising the composed required inference.
package main

import (
	"encoding/json"
	"net/http"
)

// Registration mixes required and optional fields.
type Registration struct {
	Username string `json:"username" validate:"required"` // required
	Email    string `json:"email"`                        // required (no omitempty)
	Nickname string `json:"nickname,omitempty"`           // optional
	Referral string `json:"referral,omitempty" validate:"omitempty"`
	Age      int    `json:"age,omitempty"` // optional
	Verified bool   `json:"verified"`      // required (no omitempty)
}

func register(w http.ResponseWriter, r *http.Request) {
	var reg Registration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(reg)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", register)
	_ = http.ListenAndServe(":8080", mux)
}
