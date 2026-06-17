// Package main reproduces issue #52: a multipart streaming-upload handler with
// NO JSON request body must not have one inferred from an unrelated, deep
// json.Unmarshal reachable through its call graph (here a DB-column parse). The
// Unmarshal's source ([]byte from a DB column) does not trace to r.Body, so the
// request-body inference must not attribute it to the handler.
package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler holds the upload endpoint.
type Handler struct{}

// ContentMeta is the typed view of a DB metadata column.
type ContentMeta struct {
	ImageWidth  *int `json:"imageWidth,omitempty"`
	ImageHeight *int `json:"imageHeight,omitempty"`
}

// Create is a multipart streaming upload — no JSON body, returns 201.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	h.persist(file)
	w.WriteHeader(http.StatusCreated)
}

// persist stores the upload, then reads its metadata back from the DB.
func (h *Handler) persist(rd io.Reader) {
	_, _ = io.Copy(io.Discard, rd)
	raw := h.readMetadataColumn()
	_ = parseContentMetadata(raw)
}

// readMetadataColumn returns a []byte from a database column (NOT the request).
func (h *Handler) readMetadataColumn() []byte { return []byte("{}") }

// parseContentMetadata json-unmarshals a DB column into an internal anonymous
// struct — not the HTTP request body.
func parseContentMetadata(raw []byte) ContentMeta {
	var v struct {
		ImageWidth   *int `json:"imageWidth,omitempty"`
		ImageHeight  *int `json:"imageHeight,omitempty"`
		DecodeFailed bool `json:"_decodeFailed,omitempty"`
	}
	_ = json.Unmarshal(raw, &v)
	return ContentMeta{ImageWidth: v.ImageWidth, ImageHeight: v.ImageHeight}
}

func main() {
	r := chi.NewRouter()
	h := &Handler{}
	r.Post("/internal/file", h.Create)
	_ = http.ListenAndServe(":8080", r)
}
