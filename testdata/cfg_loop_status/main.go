// Package main is the spec-009 fixture for loop-body status assignments
// (Edge Cases "Loops", FR-010). A status variable is given an unconditional
// default before a loop and conditionally reassigned inside the loop body; both
// values reach the single WriteHeader after the loop, so the CFG reachability
// model must (a) terminate across the loop back-edge and (b) report the
// loop-body assignment as reaching the response write.
package main

import "net/http"

// mapStatus wraps a status code in a call so each assignment's RHS is a call the
// status-expansion path inspects (mirrors conditional_status_reachability).
func mapStatus(s int) int { return s }

// loop sets code = 200 before the loop and code = 202 inside the loop body. The
// loop-body assignment does not dominate the WriteHeader (the loop may run zero
// times), so it does not shadow the default — the response lists BOTH 200 and 202.
func loop(w http.ResponseWriter, r *http.Request) {
	code := mapStatus(http.StatusOK)
	for _, v := range r.Header["X-Items"] {
		if v == "flag" {
			code = mapStatus(http.StatusAccepted)
		}
	}
	w.WriteHeader(code)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /loop", loop)
	_ = http.ListenAndServe(":8080", mux)
}
