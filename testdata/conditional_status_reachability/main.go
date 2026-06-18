// Package main exercises the conditional-status fan-out reachability filter
// (issue #50). The status variable is assigned from calls (mapStatus(...)) so
// the ident-based fan-out fires; the fan-out must include only the statuses
// whose assignment actually reaches the response call site.
package main

import "net/http"

// mapStatus makes the status assignment a call RHS (not a bare literal), which
// is what triggers the ident-based status fan-out.
func mapStatus(s int) int { return s }

// fanout: both if/else branches reassign code before the single trailing
// WriteHeader, so both statuses reach it — the fan-out emits 400 and 404.
func fanout(w http.ResponseWriter, r *http.Request) {
	var code int
	if r.URL.Query().Get("a") != "" {
		code = mapStatus(http.StatusBadRequest)
	} else {
		code = mapStatus(http.StatusNotFound)
	}
	w.WriteHeader(code)
}

// reachable: the only response is the 400 in the early-return branch. The later
// code = 500 assignment is positioned after that call, so it must NOT appear —
// the route is 400 only.
func reachable(w http.ResponseWriter, r *http.Request) {
	var code int
	if r.Method == http.MethodGet {
		code = mapStatus(http.StatusBadRequest)
		w.WriteHeader(code)
		return
	}
	code = mapStatus(http.StatusInternalServerError)
	_ = code
}

// shadowed: code = 500 is a dead store, overwritten unconditionally by code =
// 400 before the only WriteHeader, so only 400 reaches it — the unconditional
// reassignment shadows the earlier one. Route is 400 only.
func shadowed(w http.ResponseWriter, r *http.Request) {
	code := mapStatus(http.StatusInternalServerError)
	code = mapStatus(http.StatusBadRequest)
	w.WriteHeader(code)
}

// mixed: an unconditional default (503) followed by a conditional override
// (400); both reach the WriteHeader (503 when the branch is skipped, 400 when
// taken) — the unconditional default is not shadowed by a conditional. Route is
// 400 and 503.
func mixed(w http.ResponseWriter, r *http.Request) {
	code := mapStatus(http.StatusServiceUnavailable)
	if r.URL.Query().Get("a") != "" {
		code = mapStatus(http.StatusBadRequest)
	}
	w.WriteHeader(code)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fanout", fanout)
	mux.HandleFunc("POST /reachable", reachable)
	mux.HandleFunc("GET /shadowed", shadowed)
	mux.HandleFunc("GET /mixed", mixed)
	_ = http.ListenAndServe(":8080", mux)
}
