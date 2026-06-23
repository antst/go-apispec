// Package main exercises inline anonymous struct fields, which should describe
// their own properties rather than collapsing to an opaque object — directly, as
// a slice element, and as a fixed-array element (which also carries its length).
package main

import (
	"encoding/json"
	"net/http"
)

// Profile carries an inline anonymous struct field, a slice of inline structs,
// and a fixed-length array of inline structs.
type Profile struct {
	ID   int `json:"id"`
	Meta struct {
		CreatedBy string `json:"createdBy"`
		Version   int    `json:"version"`
	} `json:"meta"`
	Tags []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"tags"`
	Corners [2]struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"corners"`
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Profile{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /profile", getProfile)
	_ = http.ListenAndServe(":8080", mux)
}
