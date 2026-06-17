// Package main exercises gorilla/mux regex path constraints: the {id:[0-9]+}
// and {slug:[a-z-]+} templates must normalize to {id}/{slug} in the OpenAPI
// path (regex stripped) with the path params still inferred.
package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Item is the resource.
type Item struct {
	ID   int    `json:"id"`
	Slug string `json:"slug"`
}

func getItem(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["id"]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func getBySlug(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["slug"]
	w.WriteHeader(http.StatusOK)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/items/{id:[0-9]+}", getItem).Methods("GET")
	r.HandleFunc("/items/by-slug/{slug:[a-z-]+}", getBySlug).Methods("GET")
	_ = http.ListenAndServe(":8080", r)
}
