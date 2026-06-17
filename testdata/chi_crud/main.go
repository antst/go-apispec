// Package main exercises a full chi REST CRUD lifecycle using chi.URLParam and
// a render-style JSON helper, covering GET list / GET by id / POST / PATCH /
// DELETE on a single resource.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Widget is the resource.
type Widget struct {
	ID    string `json:"id"`
	Name  string `json:"name" validate:"required"`
	Color string `json:"color"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func listWidgets(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, []Widget{}) }

func getWidget(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	writeJSON(w, http.StatusOK, Widget{})
}

func createWidget(w http.ResponseWriter, r *http.Request) {
	var wg Widget
	if err := json.NewDecoder(r.Body).Decode(&wg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad"})
		return
	}
	writeJSON(w, http.StatusCreated, wg)
}

func patchWidget(w http.ResponseWriter, r *http.Request) {
	var wg Widget
	if err := json.NewDecoder(r.Body).Decode(&wg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad"})
		return
	}
	writeJSON(w, http.StatusOK, wg)
}

func deleteWidget(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }

func main() {
	r := chi.NewRouter()
	r.Get("/widgets", listWidgets)
	r.Post("/widgets", createWidget)
	r.Get("/widgets/{id}", getWidget)
	r.Patch("/widgets/{id}", patchWidget)
	r.Delete("/widgets/{id}", deleteWidget)
	_ = http.ListenAndServe(":8080", r)
}
