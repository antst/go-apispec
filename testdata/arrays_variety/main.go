// Package main exercises array fields of varied element types: []string, []int,
// a slice of structs ([]Tag) and a slice of pointers ([]*Tag).
package main

import (
	"encoding/json"
	"net/http"
)

// Tag is a struct element.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Dataset carries several array shapes.
type Dataset struct {
	Names   []string `json:"names"`
	Scores  []int    `json:"scores"`
	Tags    []Tag    `json:"tags"`
	PtrTags []*Tag   `json:"ptrTags"`
}

func getDataset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Dataset{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dataset", getDataset)
	_ = http.ListenAndServe(":8080", mux)
}
