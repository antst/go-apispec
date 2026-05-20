// Package main reproduces the issue #27 false-positive: a shared
// WriteJSON helper with a json.Marshal error fallback was polluting
// every caller's response schemas with the fallback's literal body and
// hardcoded 500 status.
package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// CheckRoomHTTPResponse is the real success-path payload.
type CheckRoomHTTPResponse struct {
	OK bool `json:"ok"`
}

// WriteJSON is the alkem-io/matrix-adapter-go shape: marshal the payload,
// fall back to a hardcoded 500 + literal body if marshalling fails. The
// fallback is practically unreachable (structs/maps don't fail to marshal)
// but its w.Write([]byte) call was being attributed to every caller's
// success-path response.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to encode response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// WriteJSONIo is the io.WriteString variant from the same bug report.
// Same shape, just uses io.WriteString instead of w.Write([]byte) — the
// reporter noted both forms get conflated into the caller's schema.
func WriteJSONIo(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"failed to encode response"}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /room/check", checkRoom)
	mux.HandleFunc("POST /room/check-io", checkRoomIo)
	http.ListenAndServe(":3000", mux)
}

// checkRoom is the bug shape: success returns CheckRoomHTTPResponse,
// failure returns a map[string]string error. Expected oneOf has exactly
// those two; the helper's internal error fallback must NOT show up.
func checkRoom(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, CheckRoomHTTPResponse{OK: true})
	WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
}

// checkRoomIo is the io.WriteString variant — same expected behaviour.
func checkRoomIo(w http.ResponseWriter, _ *http.Request) {
	WriteJSONIo(w, http.StatusOK, CheckRoomHTTPResponse{OK: true})
	WriteJSONIo(w, http.StatusBadRequest, map[string]string{"error": "invalid_payload"})
}
