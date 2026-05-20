// Package main exercises the full auth-detection matrix:
//
//   - Bearer via a shared helper (ValidateBearerToken).
//   - Bearer inline (Header.Get + TrimPrefix in the handler body).
//   - Basic via a shared helper — different scheme name, same idiom.
//   - apiKey: raw header read, no TrimPrefix → falls back to apiKey scheme.
//   - No auth at all — must stay free of any security stanza.
//
// All five live behind a net/http ServeMux so they share the HTTP-framework
// default config; the auth-detection logic runs identically across frameworks.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /protected/helper", protectedViaHelper)
	mux.HandleFunc("POST /protected/inline", protectedInline)
	mux.HandleFunc("POST /protected/basic", protectedBasic)
	mux.HandleFunc("POST /protected/apikey", protectedAPIKey)
	mux.HandleFunc("GET /open/ping", openPing)
	http.ListenAndServe(":3000", mux)
}

// PingResponse is the trivial success payload for /open/ping.
type PingResponse struct {
	Pong bool `json:"pong"`
}

// ProtectedResponse is the payload for every /protected/* endpoint.
type ProtectedResponse struct {
	OK bool `json:"ok"`
}

// ValidateBearerToken is the helper apispec should recognise — it reads
// the Authorization header, trims the "Bearer " prefix, and compares.
// Any handler that calls it should automatically get a bearer security
// scheme and lose the redundant Authorization header parameter.
func ValidateBearerToken(r *http.Request, expected string) bool {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	return token == expected
}

// ValidateBasicToken mirrors the bearer helper but for HTTP Basic auth.
// Same detection pathway, different prefix → different scheme name.
func ValidateBasicToken(r *http.Request, expected string) bool {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Basic ")
	return token == expected
}

// protectedViaHelper delegates auth to ValidateBearerToken. The
// Authorization header doesn't appear directly in this function — apispec
// has to follow the call into the helper to recognise the pattern.
func protectedViaHelper(w http.ResponseWriter, r *http.Request) {
	if !ValidateBearerToken(r, "secret") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProtectedResponse{OK: true})
}

// protectedInline does the Header.Get + TrimPrefix dance directly in the
// handler body — same security scheme should be inferred without the
// helper indirection. Both /protected/helper and /protected/inline must
// resolve to the SAME bearerAuth scheme (deduplicated under components).
func protectedInline(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProtectedResponse{OK: true})
}

// protectedBasic uses the Basic-auth helper. The generated spec should
// expose a separate basicAuth scheme alongside bearerAuth.
func protectedBasic(w http.ResponseWriter, r *http.Request) {
	if !ValidateBasicToken(r, "secret") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProtectedResponse{OK: true})
}

// protectedAPIKey reads the Authorization header as a raw token — no
// scheme prefix. Detection falls back to apiKey-in-header.
func protectedAPIKey(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProtectedResponse{OK: true})
}

// openPing has no auth and must not be associated with any security scheme.
func openPing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PingResponse{Pong: true})
}
