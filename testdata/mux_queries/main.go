// Package main exercises gorilla/mux .Queries(...) query-parameter declarations
// on the route builder chain. /search declares q and page; /list declares none
// — the params must attach only to their own route, not leak across siblings.
package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func search(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("q")
	w.WriteHeader(http.StatusOK)
}

func list(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/search", search).Methods("GET").Queries("q", "{q}", "page", "{page}")
	r.HandleFunc("/list", list).Methods("GET")
	_ = http.ListenAndServe(":8080", r)
}
