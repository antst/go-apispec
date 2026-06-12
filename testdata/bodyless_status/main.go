// Package main is the regression fixture for antst/go-apispec#33 — bodyless
// HTTP status codes (1xx, 204, 304) must produce response entries with no
// `content` map in the generated OpenAPI spec, per RFC 9110 and OpenAPI 3.1.
//
// The fixture's GET /document/{id} handler exercises four branches landing on
// four distinct status codes (200, 102, 204, 304) on the same route, ensuring
// every range of isBodylessStatusCode (1xx + 204 + 304) is covered alongside
// a body-bearing 200 control. The handler also calls Header().Set with a
// non-default Content-Type to trigger the content-type override loop (Layer-2
// guard surface).
package main

import (
	"net/http"
)

// fixedETag is the value the handler checks against If-None-Match. Hardcoded
// here so the spec generator can resolve the literal at analysis time.
const fixedETag = `"v1"`

// Handler is a method receiver to mirror real-world service shapes.
type Handler struct{}

// GetDocument has four branches landing on four distinct status codes:
//
//	?slow=1     → 102 Processing                  (bodyless 1xx)
//	?delete=1   → 204 No Content                  (bodyless)
//	matching ETag → 304 Not Modified              (bodyless)
//	default     → 200 OK with octet-stream body   (body-bearing control)
//
// The default branch also calls Header().Set("Content-Type", ...) to exercise
// the route-level content-type detection. Before the issue #33 fix, that
// detected content-type would leak onto the 1xx/204/304 entries via the
// mapper's unconditional Content emission.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("slow") == "1" {
		w.WriteHeader(http.StatusProcessing)
		return
	}
	if r.URL.Query().Get("delete") == "1" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Header.Get("If-None-Match") == fixedETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	body := []byte{0x00, 0x01, 0x02, 0x03}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func main() {
	mux := http.NewServeMux()
	h := &Handler{}
	mux.HandleFunc("GET /document/{id}", h.GetDocument)
	_ = http.ListenAndServe(":8080", mux)
}
