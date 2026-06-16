// Package main exercises deeply nested DTOs: a struct containing a struct
// containing a struct, plus a slice of nested structs — so the schema must emit
// and reference several component types in a chain.
package main

import (
	"encoding/json"
	"net/http"
)

// GeoPoint is the innermost type.
type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Address nests a GeoPoint.
type Address struct {
	Street string   `json:"street"`
	City   string   `json:"city"`
	Coords GeoPoint `json:"coords"`
}

// Company nests an Address.
type Company struct {
	Name string  `json:"name"`
	HQ   Address `json:"hq"`
}

// Person nests a Company and a slice of Addresses.
type Person struct {
	Name      string    `json:"name"`
	Employer  Company   `json:"employer"`
	Addresses []Address `json:"addresses"`
}

func createPerson(w http.ResponseWriter, r *http.Request) {
	var p Person
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /people", createPerson)
	_ = http.ListenAndServe(":8080", mux)
}
