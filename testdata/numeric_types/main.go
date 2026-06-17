// Package main exercises the full range of Go numeric scalar types (int8..int64,
// uint variants, float32/float64) and bool mapping to OpenAPI type/format.
package main

import (
	"encoding/json"
	"net/http"
)

// Metrics carries one field per numeric kind.
type Metrics struct {
	Count    int     `json:"count"`
	Small    int8    `json:"small"`
	Medium   int16   `json:"medium"`
	Large    int32   `json:"large"`
	Huge     int64   `json:"huge"`
	Unsigned uint    `json:"unsigned"`
	Byte     uint8   `json:"byteVal"`
	Ratio    float32 `json:"ratio"`
	Precise  float64 `json:"precise"`
	Enabled  bool    `json:"enabled"`
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Metrics{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", getMetrics)
	_ = http.ListenAndServe(":8080", mux)
}
