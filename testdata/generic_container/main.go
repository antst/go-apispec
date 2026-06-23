// Package main exercises a GENERIC defined-"other" type used as a struct field.
// `type Box[T any] map[string]T` is a defined type whose underlying is a map
// (Kind "other"). A field of an instantiation Box[int] must resolve through the
// underlying with the type parameter SUBSTITUTED — Box[int] -> map[string]int
// (an object with integer additionalProperties) — instead of dangling a $ref to
// the bare parameter "T". The substitution must hold for a direct field, a
// pointer field (transparent), and a slice field (array of the substituted map),
// and for a second parameter binding (Pair[K,V]).
package main

import (
	"encoding/json"
	"net/http"
)

// Box is a generic defined map type (Kind "other").
type Box[T any] map[string]T

// List is a generic defined slice type (Kind "other").
type List[T any] []T

// Holder fields each instantiate a generic defined-"other" type in a different
// position.
type Holder struct {
	Counts  Box[int]     `json:"counts"`  // direct -> object{additionalProperties: integer}
	Labels  *Box[string] `json:"labels"`  // pointer -> object{additionalProperties: string}
	Batches []Box[bool]  `json:"batches"` // slice -> array of object{additionalProperties: boolean}
	Names   List[string] `json:"names"`   // direct generic slice -> array of string
	Nested  []List[int]  `json:"nested"`  // slice of generic slice -> array of array of integer
}

func getHolder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Holder{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /holder", getHolder)
	_ = http.ListenAndServe(":8080", mux)
}
