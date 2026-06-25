// Package main is the spec-009 guard for a method dispatch SPLIT ACROSS an
// `if r.Method ==` arm AND a `switch r.Method` with a `default`. The two dispatches are
// DISTINCT groups (the GET response is attributed to the if group, POST to the switch
// group). The dispatch root must be the common dominator of the arms of BOTH contributing
// groups — so the switch's `default` 405 is still dominated and excluded, not leaked onto
// the GET and POST operations. Scoping the root to a single "primary" group regressed this
// (the if group's lone arm does not dominate the switch default) — the union of contributing
// groups fixes it.
package main

import (
	"encoding/json"
	"net/http"
)

// GetItem is the GET response body.
type GetItem struct {
	ID int `json:"id"`
}

// PostItem is the POST response body.
type PostItem struct {
	Name string `json:"name"`
}

func items(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(GetItem{ID: 1})
		return
	}
	switch r.Method {
	case http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PostItem{Name: "x"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
