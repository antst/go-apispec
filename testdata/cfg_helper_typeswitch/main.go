// Package main is the spec-009 fixture for helper-internal type-switch binding
// (FR-011/FR-012). A shared Respond(w, x) helper type-switches on the concrete
// type of x to choose status + body. The analyzer must bind the call-site
// argument to the matching arm:
//
//   - GET /concrete passes a statically-known *NotFoundError, so ONLY the
//     `case *NotFoundError` arm (404) fans out — not the *SuccessBody (200) or
//     default (500) arms.
//   - GET /imprecise passes a value whose concrete type is not statically known
//     (an `any` holding an error), so the binding is imprecise: the analyzer
//     degrades to the unconditionally-reachable default arm (500) and warns,
//     rather than over-approximating with every arm.
package main

import (
	"encoding/json"
	"net/http"
)

// NotFoundError is the 404 payload.
type NotFoundError struct {
	Resource string `json:"resource"`
}

// SuccessBody is the 200 payload.
type SuccessBody struct {
	Message string `json:"message"`
}

// DefaultError is the fallback (default arm) payload.
type DefaultError struct {
	Code int `json:"code"`
}

// Respond chooses the response status and body from the concrete type of x.
func Respond(w http.ResponseWriter, x any) {
	switch v := x.(type) {
	case *NotFoundError:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(v)
	case *SuccessBody:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(v)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(&DefaultError{Code: 500})
	}
}

// process stands in for work that yields an error of statically-unknown concrete
// type at the call site.
func process(r *http.Request) error { return nil }

// handleConcrete passes a statically-known *NotFoundError → only 404.
func handleConcrete(w http.ResponseWriter, r *http.Request) {
	Respond(w, &NotFoundError{Resource: "user"})
}

// handleImprecise passes a bare error (as any) whose concrete type is unknown
// here → degrade to the default arm (500) + warn.
func handleImprecise(w http.ResponseWriter, r *http.Request) {
	var x any = process(r)
	Respond(w, x)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /concrete", handleConcrete)
	mux.HandleFunc("GET /imprecise", handleImprecise)
	_ = http.ListenAndServe(":8080", mux)
}
