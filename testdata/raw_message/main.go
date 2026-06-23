// Package main exercises json.RawMessage handling. RawMessage's underlying is
// []byte, but it implements json.Marshaler and round-trips RAW JSON, so its
// schema is the empty schema {} (any JSON value), NOT the base64 string a plain
// []byte slice produces. The empty schema must compose through a pointer (-> {})
// and a slice (-> array of {}). A plain []byte field is included for contrast and
// must still be a base64 string.
package main

import (
	"encoding/json"
	"net/http"
)

// Envelope carries RawMessage in direct, pointer, and slice positions plus a
// plain []byte for contrast.
type Envelope struct {
	Payload  json.RawMessage   `json:"payload"`  // arbitrary JSON -> {}
	Optional *json.RawMessage  `json:"optional"` // pointer is transparent -> {}
	Items    []json.RawMessage `json:"items"`    // array of arbitrary JSON -> array of {}
	Blob     []byte            `json:"blob"`     // plain []byte -> base64 string (contrast)
}

func getEnvelope(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Envelope{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /envelope", getEnvelope)
	_ = http.ListenAndServe(":8080", mux)
}
