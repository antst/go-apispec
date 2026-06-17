// Package main exercises Go field promotion via anonymous struct embedding,
// including a pointer embed and transitive (embed-of-embed) promotion. All
// promoted fields must appear flat on the outer schema.
package main

import (
	"encoding/json"
	"net/http"
)

// Base is embedded by value.
type Base struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

// Audit is embedded by pointer.
type Audit struct {
	UpdatedBy string `json:"updatedBy"`
}

// Document embeds Base (value) and Audit (pointer) plus an own field.
type Document struct {
	Base
	*Audit
	Title string `json:"title"`
}

// DetailedDocument transitively embeds Document, so Base/Audit fields promote
// through two levels.
type DetailedDocument struct {
	Document
	Description string `json:"description"`
}

func getDoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Document{})
}

func getDetailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DetailedDocument{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /doc", getDoc)
	mux.HandleFunc("GET /detailed", getDetailed)
	_ = http.ListenAndServe(":8080", mux)
}
