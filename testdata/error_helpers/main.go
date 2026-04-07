// Package main demonstrates error helper function patterns for testing
// apispec's ability to detect response types and status codes passed
// through helper functions via parameter propagation.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// User represents a user entity.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Item represents an item entity.
type Item struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Error helper functions ---

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Code: code})
}

// respondJSON writes a JSON success response with the given status code.
func respondJSON(w http.ResponseWriter, code int, data interface{}) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func main() {
	r := chi.NewRouter()

	r.Get("/users/{id}", GetUser)
	r.Post("/users", CreateUser)
	r.Put("/users/{id}", UpdateUser)
	r.Get("/items", ListItems)

	http.ListenAndServe(":3000", r)
}

// GetUser returns a user by ID, or 400/404 errors via helper.
func GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id parameter")
		return
	}
	if id == "0" {
		writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}
	json.NewEncoder(w).Encode(User{ID: 1, Name: "Alice"})
}

// CreateUser creates a new user, or returns 400/413 errors via helper.
func CreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if user.Name == "" {
		writeJSONError(w, http.StatusUnprocessableEntity, "name is required")
		return
	}
	respondJSON(w, http.StatusCreated, user)
}

// ListItems returns all items, or 500 error via helper.
func ListItems(w http.ResponseWriter, r *http.Request) {
	items := []Item{{ID: 1, Title: "Widget"}}
	if len(items) == 0 {
		writeJSONError(w, http.StatusInternalServerError, "failed to load items")
		return
	}
	json.NewEncoder(w).Encode(items)
}

// UpdateUser uses respondJSON for BOTH success and error with different types.
// This tests that sibling calls to the same helper get their own schema
// based on their own arguments, not copied from the first call.
func UpdateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		respondJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid body"})
		return
	}
	respondJSON(w, http.StatusOK, user)
}
