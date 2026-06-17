// Package main exercises apispec format tags (uuid, email, date-time, uri, ipv4,
// hostname) applied to plain string fields — the explicit format-inference path.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Profile carries one field per apispec format.
type Profile struct {
	ID       string `json:"id" apispec:"format=uuid"`
	Email    string `json:"email" apispec:"format=email"`
	JoinedAt string `json:"joinedAt" apispec:"format=date-time"`
	Website  string `json:"website" apispec:"format=uri"`
	IP       string `json:"ip" apispec:"format=ipv4"`
	Host     string `json:"host" apispec:"format=hostname"`
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Profile{})
}

func main() {
	r := chi.NewRouter()
	r.Get("/profiles/{id}", getProfile)
	_ = http.ListenAndServe(":8080", r)
}
