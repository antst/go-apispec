// Package main exercises a wide spread of HTTP status codes — 201/202/204 plus
// error statuses 400/404/409/422 written through a shared error helper — so the
// generated responses map cover more than the usual 200/201/400 trio.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Job is the resource.
type Job struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// APIError is the shared error body.
type APIError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{Message: msg, Code: code})
}

// submitJob returns 202 Accepted.
func submitJob(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(Job{})
}

// createJob returns 201, or 409 on conflict / 422 on invalid input.
func createJob(w http.ResponseWriter, r *http.Request) {
	var j Job
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "bad payload")
		return
	}
	if j.ID == "dup" {
		writeError(w, http.StatusConflict, "conflict", "already exists")
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(j)
}

// getJob returns 200 or 404.
func getJob(w http.ResponseWriter, r *http.Request) {
	if chi.URLParam(r, "id") == "" {
		writeError(w, http.StatusNotFound, "not_found", "no such job")
		return
	}
	_ = json.NewEncoder(w).Encode(Job{})
}

// cancelJob returns 204 No Content.
func cancelJob(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	r := chi.NewRouter()
	r.Post("/jobs", createJob)
	r.Post("/jobs/submit", submitJob)
	r.Get("/jobs/{id}", getJob)
	r.Delete("/jobs/{id}", cancelJob)
	_ = http.ListenAndServe(":8080", r)
}
