// Package main exercises gorilla/mux subrouters (PathPrefix().Subrouter()) with
// per-route .Methods() matchers and {id} path variables — the idiomatic mux
// nesting the flat mux fixture doesn't cover.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// Repo is the resource.
type Repo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Stars int    `json:"stars"`
}

func listRepos(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Repo{})
}

func getRepo(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["id"]
	_ = json.NewEncoder(w).Encode(Repo{})
}

func createRepo(w http.ResponseWriter, r *http.Request) {
	var repo Repo
	if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(repo)
}

func main() {
	r := mux.NewRouter()
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/repos", listRepos).Methods("GET")
	api.HandleFunc("/repos", createRepo).Methods("POST")
	api.HandleFunc("/repos/{id}", getRepo).Methods("GET")
	_ = http.ListenAndServe(":8080", r)
}
