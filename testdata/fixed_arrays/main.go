// Package main exercises fixed-length array fields, which (unlike slices) carry
// their length as an OpenAPI constraint: a [N]T array emits minItems == maxItems
// == N. A [N]byte ARRAY is no exception — it marshals as a JSON array of N
// integers (only a []byte SLICE is a base64 string), and a plain []T slice (for
// contrast) stays unconstrained.
package main

import (
	"encoding/json"
	"net/http"
)

// Point is a struct element used inside a fixed-length array.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Shape carries fixed-length arrays of varied element types alongside an
// unconstrained slice.
type Shape struct {
	Hash     [16]byte `json:"hash"`     // array of 16 integers (byte ARRAY, not base64)
	Scores   [3]int   `json:"scores"`   // array, minItems == maxItems == 3
	Vertices [4]Point `json:"vertices"` // array of $ref, minItems == maxItems == 4
	Tags     []string `json:"tags"`     // unconstrained array (contrast)
}

func getShape(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Shape{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /shape", getShape)
	_ = http.ListenAndServe(":8080", mux)
}
