// Package main demonstrates nested http.ServeMux patterns for testing
// apispec's ability to detect hierarchical route mounting in net/http.
package main

import (
	"encoding/json"
	"net/http"
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

// HealthStatus represents a health check response.
type HealthStatus struct {
	Status string `json:"status"`
}

// GlobalInfo represents info from a package-level route.
type GlobalInfo struct {
	Version string `json:"version"`
}

func main() {
	// Level 3: v1 mux with routes registered via helper function
	v1Mux := http.NewServeMux()
	registerV1Routes(v1Mux)

	// Level 2: api mux — mounts v1 with StripPrefix wrapping
	apiMux := http.NewServeMux()
	apiMux.Handle("/v1/", http.StripPrefix("/v1", v1Mux))
	apiMux.HandleFunc("/health", APIHealth)

	// Level 1: root mux — mounts api with StripPrefix wrapping
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", http.StripPrefix("/api", apiMux))
	rootMux.HandleFunc("/ping", Ping)

	// Package-level route on DefaultServeMux (FR-009)
	http.HandleFunc("/global", GlobalHandler)

	http.ListenAndServe(":8080", rootMux)
}

// registerV1Routes registers routes on the v1 mux via a helper function.
// Tests variable-based mux passing (US3).
func registerV1Routes(mux *http.ServeMux) {
	mux.HandleFunc("/users", ListUsers)
	mux.HandleFunc("/items", ListItems)
	// Edge case: /health also exists on apiMux — tests same path at different levels
	mux.HandleFunc("/health", V1Health)
}

// --- Handlers ---

// ListUsers returns a list of users.
func ListUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{{ID: 1, Name: "Alice"}}
	json.NewEncoder(w).Encode(users)
}

// ListItems returns a list of items.
func ListItems(w http.ResponseWriter, r *http.Request) {
	items := []Item{{ID: 1, Title: "Widget"}}
	json.NewEncoder(w).Encode(items)
}

// APIHealth returns API health status.
func APIHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(HealthStatus{Status: "ok"})
}

// V1Health returns v1-specific health status.
func V1Health(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(HealthStatus{Status: "v1-ok"})
}

// Ping returns a simple text response.
func Ping(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

// GlobalHandler returns global info (registered on DefaultServeMux).
func GlobalHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(GlobalInfo{Version: "1.0"})
}
