// Package main exercises native time.Time fields (and a pointer time.Time) on
// request/response DTOs, which should map to OpenAPI string/date-time without
// any explicit apispec format tag.
package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// Event carries time.Time fields directly (no format tag) plus a pointer time.
type Event struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	StartsAt  time.Time  `json:"startsAt"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

func createEvent(w http.ResponseWriter, r *http.Request) {
	var e Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

func getEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Event{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", createEvent)
	mux.HandleFunc("GET /events/{id}", getEvent)
	_ = http.ListenAndServe(":8080", mux)
}
