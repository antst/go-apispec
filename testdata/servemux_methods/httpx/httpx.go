// Package httpx holds the shared response helper. Living in a sibling
// package — not in main — matters because the bug we're chasing involves
// cross-package type resolution where the request body's type ends up
// confused with one of the helper's interface{} arguments.
package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON is the shared response writer used by every handler. The
// interface{} parameter is intentional — `v` accepts any value the handler
// wants to serialize. apispec needs to follow the concrete call-site arg
// type back through this helper, not pick something up from the handler's
// local scope.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
