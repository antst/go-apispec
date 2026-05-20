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
	"servemux_methods/httpx"
)

func main() {
	mux := http.NewServeMux()
	h := &CheckRoomHandler{rpc: &rpcClient{}}

	// Go 1.22+ method-prefix syntax: GET-only liveness probe.
	mux.HandleFunc("GET /health/live", liveness)

	// Go 1.22+ method-prefix syntax: POST with request body, method receiver.
	mux.HandleFunc("POST /matrix/check-room", h.handleCheckRoom)

	// Plain pattern (no method prefix) — the legacy path should still work.
	mux.HandleFunc("/legacy", legacy)

	http.ListenAndServe(":3000", mux)
}

// rpcClient is a stand-in for an injected RPC dependency. The CheckRoom
// method does its own JSON deserialization (json.Unmarshal) — that
// internal decode of a RabbitMQ-style byte payload must NOT be picked
// up by apispec's request-body extraction and substituted for the
// HTTP handler's real body type. This is the exact failure shape from
// issue #22.
type rpcClient struct{}

func (r *rpcClient) CheckRoom(req dto.CheckRoomRequest) (dto.CheckRoomResponse, error) {
	// Pretend we received this from the message broker.
	respBytes := []byte(`{"allowed":true}`)
	var rmqResp dto.CheckRoomResponse
	if err := json.Unmarshal(respBytes, &rmqResp); err != nil {
		return dto.CheckRoomResponse{}, err
	}
	_ = req
	return rmqResp, nil
}

// CheckRoomHandler holds dependencies for the /matrix/check-room handler.
// Matches the reporter's project — handler is a method on a struct with an
// injected RPC client.
type CheckRoomHandler struct {
	rpc *rpcClient
}

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

	// Internal RPC call — returns a CheckRoomResponse, which is the
	// look-alike type the issue-#22 bug confused with the request body.
	rpcReq := dto.CheckRoomRequest{RoomID: httpReq.Room}
	rpcResp, err := h.rpc.CheckRoom(rpcReq)
	if err != nil {
		http.Error(w, "rpc failed", http.StatusInternalServerError)
		return
	}

	// Final HTTP response value.
	httpResp := dto.CheckRoomHTTPResponse{OK: rpcResp.Allowed}
	httpx.WriteJSON(w, http.StatusOK, httpResp)
}

// liveness returns a fixed JSON payload — never reads the body.
func liveness(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, dto.LivenessResponse{Status: "ok"})
}

// legacy is registered without a method prefix; defaults still apply.
func legacy(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, dto.LivenessResponse{Status: "legacy"})
}
