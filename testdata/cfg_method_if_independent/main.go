// Package main is the spec-009 fixture for response attribution ACROSS a method
// dispatch (US2). The handler combines an `if r.Method ==` dispatch (GET lists,
// POST creates) with an INDEPENDENT pre-dispatch guard (`if bad { 500 }`) that is
// reachable whatever the request method is. The analyzer must split into one
// operation per method AND carry the independent 500 onto BOTH operations — the
// CFG tells it the 500 is orthogonal to the dispatch, not a method-specific or
// "other-methods" response. (Before the CFG model, splitting dropped it entirely.)
package main

import (
	"encoding/json"
	"net/http"
)

// ErrBody is the independent (method-independent) error response body.
type ErrBody struct {
	Msg string `json:"msg"`
}

// ItemList is the GET response body.
type ItemList struct {
	Items []string `json:"items"`
}

// CreatedItem is the POST response body.
type CreatedItem struct {
	ID int `json:"id"`
}

func items(w http.ResponseWriter, r *http.Request) {
	// Independent of the method dispatch: a bad request fails on ANY method.
	if r.URL.Query().Get("bad") != "" {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ErrBody{Msg: "invalid"})
		return
	}
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(ItemList{Items: []string{"a"}})
	} else if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreatedItem{ID: 1})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
