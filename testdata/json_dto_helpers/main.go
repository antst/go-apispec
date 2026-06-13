// Package main is the interprocedural counterpart of the json_dto fixture: it
// exercises the same request-body inference (requestBody.required, per-field
// format: uuid) but after the lint-driven helper extractions described in
// issue #36, where the literal patterns no longer sit inside the handler body.
//
// It guards three interprocedural shapes:
//
//   - Repro 2 — the strict-decode boilerplate is extracted into decodeStrictJSON,
//     whose decode target is an `any` parameter. Binding must follow the call
//     site to recover the concrete CopyDocumentRequest type.
//   - Repro 1 (field-passed) — the handler passes body.SourceID into a helper
//     that runs uuid.Parse on it; format: uuid must propagate back to sourceId.
//   - Repro 1 (struct-passed) — the handler passes the whole body into a helper
//     that selects fields and runs uuid.Parse on them; format: uuid must
//     propagate back to destinationBucketId, authorizationId and tagsetId.
//
// The generated schema is expected to match the json_dto fixture's
// CopyDocumentRequest byte-for-byte (modulo operationId/summary).
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// CopyDocumentRequest mirrors the json_dto fixture: string-typed UUID fields
// validated at runtime via uuid.Parse, plus two tag-pinned fields.
type CopyDocumentRequest struct {
	SourceID            string  `json:"sourceId"`
	DestinationBucketID string  `json:"destinationBucketId"`
	AuthorizationID     string  `json:"authorizationId"`
	TagsetID            *string `json:"tagsetId,omitempty"`

	ExternalID string `json:"externalId" apispec:"format=uuid"`
	ExpiresAt  string `json:"expiresAt" apispec:"format=date-time"`
}

// CopyDocumentResponse is returned on success; its fields are pinned via tags.
type CopyDocumentResponse struct {
	ID         string `json:"id" apispec:"format=uuid"`
	CreatedAt  string `json:"createdAt" apispec:"format=date-time"`
	OwnerEmail string `json:"ownerEmail" apispec:"format=email"`
}

func main() {
	r := chi.NewRouter()
	r.Post("/documents/copy", Copy)
	http.ListenAndServe(":3000", r)
}

// decodeStrictJSON is the extracted strict-decode helper (issue #36, Repro 2).
// dst is typed any, so the decode target type is only knowable at the call site.
func decodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	return true
}

// parseSourceID is a field-passed helper: it receives a single field's value
// (body.SourceID) and runs uuid.Parse on it (issue #36, Repro 1).
func parseSourceID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

// resolveBucketIDs is a struct-passed helper: it receives the whole request
// struct and selects fields to validate as UUIDs (issue #36, Repro 1).
func resolveBucketIDs(body CopyDocumentRequest) error {
	if _, err := uuid.Parse(body.DestinationBucketID); err != nil {
		return err
	}
	if _, err := uuid.Parse(body.AuthorizationID); err != nil {
		return err
	}
	if body.TagsetID != nil {
		if _, err := uuid.Parse(*body.TagsetID); err != nil {
			return err
		}
	}
	return nil
}

// Copy decodes and validates the request body entirely through helpers.
func Copy(w http.ResponseWriter, r *http.Request) {
	var body CopyDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}

	if _, err := parseSourceID(body.SourceID); err != nil {
		http.Error(w, "invalid sourceId", http.StatusBadRequest)
		return
	}
	if err := resolveBucketIDs(body); err != nil {
		http.Error(w, "invalid bucket id", http.StatusBadRequest)
		return
	}

	resp := CopyDocumentResponse{
		ID:         uuid.NewString(),
		CreatedAt:  time.Now().Format(time.RFC3339),
		OwnerEmail: "owner@example.com",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
