// Package main exercises nested slice types [][]T: the inner slice must recurse
// as a nested array (items: {type: array, items: …}) rather than being
// name-mangled into a bogus _T component reference.
package main

import (
	"encoding/json"
	"net/http"
)

// Cell is the matrix element struct.
type Cell struct {
	Value int `json:"value"`
}

// Grid holds nested-slice fields of primitives and structs.
type Grid struct {
	Matrix  [][]int    `json:"matrix"`
	Cells   [][]Cell   `json:"cells"`
	Strings [][]string `json:"strings"`
}

func getGrid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Grid{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /grid", getGrid)
	_ = http.ListenAndServe(":8080", mux)
}
