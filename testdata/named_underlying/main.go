// Package main exercises DEFINED types whose underlying is not a
// struct/alias/interface — a defined map, slice, fixed/nested array, and
// function. Such a type is classified Kind "other" by the analyzer, and a
// struct field of one previously emitted a dangling $ref (regression). Each
// field's schema must now derive from the captured underlying shape: a defined
// map -> object+additionalProperties, a defined slice -> array, a defined
// nested-slice -> array-of-array, while a defined func type is opaque and the
// field is omitted entirely (NOT a dangling $ref).
package main

import (
	"encoding/json"
	"net/http"
)

// Tags is a defined map type (underlying map[string]string).
type Tags map[string]string

// Codes is a defined slice type (underlying []int).
type Codes []int

// Matrix is a defined nested-slice type (underlying [][]float64).
type Matrix [][]float64

// Handler is a defined function type (opaque — yields no schema).
type Handler func()

// Record carries fields of each defined "other"-kind type, including a POINTER to
// one and two CONTAINERS of one, so the same defined type resolves via several
// paths — direct inline, pointer inline, and the schemaForOtherKind element path —
// and must yield the FULL underlying schema every time, independent of field
// order. The Handler field must be dropped, leaving no dangling $ref behind.
type Record struct {
	Tags     Tags            `json:"tags"`     // object, additionalProperties: string
	TagsPtr  *Tags           `json:"tagsPtr"`  // pointer is transparent -> same object schema
	TagsList []Tags          `json:"tagsList"` // array of (object, additionalProperties: string)
	TagsMap  map[string]Tags `json:"tagsMap"`  // object whose values are (object, additionalProperties: string)
	Codes    Codes           `json:"codes"`    // array, items: integer
	Matrix   Matrix          `json:"matrix"`   // array of array of number
	OnEvent  Handler         `json:"onEvent"`  // omitted (opaque func underlying)
}

func getRecord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Record{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /record", getRecord)
	_ = http.ListenAndServe(":8080", mux)
}
