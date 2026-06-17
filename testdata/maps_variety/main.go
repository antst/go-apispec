// Package main exercises map fields with scalar value types — map[string]string,
// map[string]int, map[string]bool — each rendering as an object with the matching
// additionalProperties scalar.
package main

import (
	"encoding/json"
	"net/http"
)

// Config carries several scalar-valued map shapes.
type Config struct {
	Labels map[string]string `json:"labels"`
	Counts map[string]int    `json:"counts"`
	Flags  map[string]bool   `json:"flags"`
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Config{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", getConfig)
	_ = http.ListenAndServe(":8080", mux)
}
