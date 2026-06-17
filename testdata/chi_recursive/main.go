// Package main exercises self-referential (recursive) types — a category tree
// whose Children field is a slice of the same type, plus a parent pointer — so
// the schema must emit a $ref back to itself without infinite recursion.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Category is a recursive tree node: Children are Categories, Parent points to
// one (a nullable self-reference).
type Category struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Children []Category `json:"children"`
	Parent   *Category  `json:"parent,omitempty"`
}

// Comment is recursive through a pointer slice (threaded replies).
type Comment struct {
	ID      string     `json:"id"`
	Body    string     `json:"body"`
	Replies []*Comment `json:"replies"`
}

func getCategory(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	_ = json.NewEncoder(w).Encode(Category{})
}

func getComment(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Comment{})
}

func main() {
	r := chi.NewRouter()
	r.Get("/categories/{id}", getCategory)
	r.Get("/comments/{id}", getComment)
	_ = http.ListenAndServe(":8080", r)
}
