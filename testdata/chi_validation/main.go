// Package main exercises go-playground/validator numeric & string bound tags:
// gte/lte must map to minimum/maximum on numeric fields, and min/max (and
// gte/lte) on a string field denote its length → minLength/maxLength.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// CreateUser is the request body.
type CreateUser struct {
	Age      int    `json:"age" validate:"gte=0,lte=120"`
	Score    int    `json:"score" validate:"min=1,max=10"`
	Username string `json:"username" validate:"min=3,max=20"`
	Nickname string `json:"nickname" validate:"gte=2,lte=15"`
	Email    string `json:"email" validate:"required,email"`
}

func create(w http.ResponseWriter, r *http.Request) {
	var u CreateUser
	_ = json.NewDecoder(r.Body).Decode(&u)
	w.WriteHeader(http.StatusCreated)
}

func main() {
	r := chi.NewRouter()
	r.Post("/users", create)
	_ = http.ListenAndServe(":8080", r)
}
