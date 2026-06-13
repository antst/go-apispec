// Package main guards issue #39 Variant B: a generic strict-decode helper
// `decodeStrictJSON[T any](..., dst *T)`, shared by two handlers. The type
// parameter lets struct identities resolve per call site, but the per-field
// format: uuid enrichment must also survive the generic boundary — it is
// re-anchored onto each handler's frame so the handler's uuid.Parse calls are
// seen.
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

// decodeStrictJSON is the shared generic decode helper called by both handlers.
func decodeStrictJSON[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
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
