// Package main exercises Go 1.22+ net/http ServeMux method-prefix patterns:
//
//	mux.HandleFunc("GET /health/live", ...)
//	mux.HandleFunc("POST /matrix/check-room", ...)
//
// Before Go 1.22, the first argument was just the path. Go 1.22+ accepts an
// optional method prefix separated by a space. apispec must split the prefix
// out so the OpenAPI path key stays `/health/live` (not `/GET /health/live`)
// and the operation verb tracks the prefix (`get:`, `post:`).
//
// The fixture also defines a handler whose body type differs from the
// response type with a name-collision-heavy `dto` package: CheckRoomRequest,
// CheckRoomResponse, CheckRoomHTTPRequest, CheckRoomHTTPResponse. The
// requestBody schema must reference dto.CheckRoomHTTPRequest, not any of
// the look-alike Response types. The handler is a method on a struct to
// mirror the reporter's setup.
package main

import (
	"encoding/json"
	"net/http"

	"servemux_methods/dto"
)

func main() {
	mux := http.NewServeMux()
	h := &CheckRoomHandler{}

	// Go 1.22+ method-prefix syntax: GET-only liveness probe.
	mux.HandleFunc("GET /health/live", liveness)

	// Go 1.22+ method-prefix syntax: POST with request body, method receiver.
	mux.HandleFunc("POST /matrix/check-room", h.handleCheckRoom)

	// Plain pattern (no method prefix) — the legacy path should still work.
	mux.HandleFunc("/legacy", legacy)

	http.ListenAndServe(":3000", mux)
}

// CheckRoomHandler holds dependencies for the /matrix/check-room handler.
// In the reporter's project it had an injected RPC client; here we just
// keep the receiver so the call shape matches.
type CheckRoomHandler struct{}

// handleCheckRoom decodes the body into dto.CheckRoomHTTPRequest, talks to
// an internal RPC layer (whose Request/Response types share a confusing
// "CheckRoom" prefix with the HTTP ones), and writes back the
// dto.CheckRoomHTTPResponse. The bug surfaces when apispec follows the
// type tracer through the RPC variables and reports CheckRoomResponse as
// the request body.
func (h *CheckRoomHandler) handleCheckRoom(w http.ResponseWriter, r *http.Request) {
	var httpReq dto.CheckRoomHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&httpReq); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Internal RPC step — these variables have the look-alike non-HTTP types.
	rpcReq := dto.CheckRoomRequest{RoomID: httpReq.Room}
	rpcResp := dto.CheckRoomResponse{Allowed: rpcReq.RoomID != ""}

	// Final HTTP response value.
	httpResp := dto.CheckRoomHTTPResponse{OK: rpcResp.Allowed}
	WriteJSON(w, http.StatusOK, httpResp)
}

// WriteJSON is a generic response helper of the kind common in real apps;
// its presence has historically confused the response type tracer.
func WriteJSON[T any](w http.ResponseWriter, status int, v T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// liveness returns a fixed JSON payload — never reads the body.
func liveness(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, dto.LivenessResponse{Status: "ok"})
}

// legacy is registered without a method prefix; defaults still apply.
func legacy(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, dto.LivenessResponse{Status: "legacy"})
}
