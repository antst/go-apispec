// Package main exercises pointer fields: pointer scalars (*string, *int, *bool),
// a pointer to a struct, and a pointer-to-slice — all optional via omitempty.
package main

import (
	"encoding/json"
	"net/http"
)

// Inner is a pointed-to struct.
type Inner struct {
	Code string `json:"code"`
}

// Patch is a partial-update DTO where every field is an optional pointer.
type Patch struct {
	Name    *string   `json:"name,omitempty"`
	Count   *int      `json:"count,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
	Detail  *Inner    `json:"detail,omitempty"`
	Items   *[]string `json:"items,omitempty"`
}

func applyPatch(w http.ResponseWriter, r *http.Request) {
	var p Patch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /resource/{id}", applyPatch)
	_ = http.ListenAndServe(":8080", mux)
}
