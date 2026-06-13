// Package main guards issue #39 Variant A: a strict-decode helper typed
// `dst any`, shared by two handlers with *different* concrete request DTOs.
// Each endpoint's requestBody must $ref its own DTO and keep the per-field
// format: uuid that flow analysis derives from the handler's uuid.Parse calls —
// rather than both endpoints collapsing onto a single body type.
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

func main() {
	r := chi.NewRouter()
	r.Post("/copy", Copy)
	r.Post("/update", Update)
	http.ListenAndServe(":3000", r)
}

// decodeStrictJSON is the shared any-typed decode helper called by both handlers.
func decodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return false
	}
	return true
}

func Copy(w http.ResponseWriter, r *http.Request) {
	var body CopyDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	_, _ = uuid.Parse(body.SourceID)
	_, _ = uuid.Parse(body.DestinationBucketID)
	w.WriteHeader(http.StatusCreated)
}

func Update(w http.ResponseWriter, r *http.Request) {
	var body UpdateDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	_, _ = uuid.Parse(body.StorageBucketID)
	w.WriteHeader(http.StatusOK)
}
