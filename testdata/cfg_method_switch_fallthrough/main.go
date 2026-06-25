// Package main is the spec-009 guard for a `fallthrough` into a `switch r.Method`
// `default:` arm. The POST arm falls through into the default (405) — so go/cfg gives
// the default body a successor edge FROM the POST arm, defeating a pure
// mutual-exclusivity test. The default is still recognised structurally (a switch-case
// arm with empty case values) and excluded, so the 405 does NOT leak onto GET or POST.
// (fallthrough between HTTP-method cases is pathological, but must not corrupt output.)
package main

import (
	"encoding/json"
	"net/http"
)

// ItemList is the GET response body.
type ItemList struct {
	Items []string `json:"items"`
}

// CreatedItem is the POST response body.
type CreatedItem struct {
	ID int `json:"id"`
}

// ErrBody is the default-arm 405 body.
type ErrBody struct {
	Msg string `json:"msg"`
}

func items(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		_ = json.NewEncoder(w).Encode(ItemList{Items: []string{"a"}})
	case "POST":
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreatedItem{ID: 1})
		fallthrough
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(ErrBody{Msg: "nope"})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", items)
	_ = http.ListenAndServe(":8080", mux)
}
