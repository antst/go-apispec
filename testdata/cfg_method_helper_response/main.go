// Package main is the spec-009 guard for cross-function response attribution. The
// GET arm writes a direct 200 (so the handler splits per method) AND calls a helper
// that writes a CONDITIONAL response. That helper response's CFG branch belongs to
// the HELPER's function, not the handler's, so its block index is meaningless in the
// handler's CFG. The classifier must NOT reason about it against the handler's CFG
// (which would alias an unrelated block and leak the response onto POST); it is
// conservatively excluded. Correct per-method attribution of an interprocedural
// conditional needs call-site context — a separate, future concern.
package main

import (
	"encoding/json"
	"net/http"
)

// List is the GET response body.
type List struct {
	Items []string `json:"items"`
}

// Trace is the helper's conditional response body (GET-only in source).
type Trace struct {
	T string `json:"t"`
}

// Created is the POST response body.
type Created struct {
	OK bool `json:"ok"`
}

// auditGet writes a CONDITIONAL response; its branch block index is local to auditGet.
func auditGet(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Trace") != "" {
		w.WriteHeader(599)
		_ = json.NewEncoder(w).Encode(Trace{T: "x"})
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(List{Items: []string{"a"}})
		auditGet(w, r) // helper's conditional 599 must NOT leak onto POST
	case "POST":
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Created{OK: true})
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/things", handler)
	_ = http.ListenAndServe(":8080", mux)
}
