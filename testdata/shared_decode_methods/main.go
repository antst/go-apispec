// Package main guards issue #41: the any-typed shared decode helper, but with
// the handlers declared as METHODS on a handler struct (as in real codebases
// such as alkem-io/file-service) rather than free functions. The per-call-site
// resolution must recognise a method handler — matched by the caller's base ID,
// not just a bare/package-qualified function name — so each endpoint keeps its
// own request DTO and per-field format: uuid.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type CopyDocumentRequest struct {
	SourceID            string `json:"sourceId"`
	DestinationBucketID string `json:"destinationBucketId"`
}

type UpdateDocumentRequest struct {
	StorageBucketID string `json:"storageBucketId"`
	DisplayName     string `json:"displayName"`
}

// DocumentHandler carries the route handlers as methods.
type DocumentHandler struct{}

func main() {
	h := &DocumentHandler{}
	r := chi.NewRouter()
	r.Post("/copy", h.Copy)
	r.Post("/update", h.Update)
	http.ListenAndServe(":3000", r)
}

// decodeStrictJSON is the shared any-typed decode helper called by both methods.
func decodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *DocumentHandler) Copy(w http.ResponseWriter, r *http.Request) {
	var body CopyDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	_, _ = uuid.Parse(body.SourceID)
	_, _ = uuid.Parse(body.DestinationBucketID)
	w.WriteHeader(http.StatusCreated)
}

func (h *DocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	var body UpdateDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	_, _ = uuid.Parse(body.StorageBucketID)
	w.WriteHeader(http.StatusOK)
}
