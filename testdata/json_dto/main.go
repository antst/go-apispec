// Package main exercises JSON request-body inference: requestBody.required
// emission and per-field schema typing for fields whose JSON value is later
// fed through a known converter (uuid.Parse, time.Parse, etc.) in the same
// handler. Includes a struct-tag fallback for cases the flow analysis can't
// catch.
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// CopyDocumentRequest is a typical multi-UUID-field request body. The fields
// are declared `string` because the wire format is JSON-string, but the
// handler validates each as a UUID via uuid.Parse — flow analysis should
// back-propagate format: uuid to the schema field.
type CopyDocumentRequest struct {
	SourceID            string  `json:"sourceId"`
	DestinationBucketID string  `json:"destinationBucketId"`
	AuthorizationID     string  `json:"authorizationId"`
	TagsetID            *string `json:"tagsetId,omitempty"`

	// Field never consumed by a converter — flow analysis can't help here, so
	// the user opts in via the apispec struct tag.
	ExternalID string `json:"externalId" apispec:"format=uuid"`

	// Mixed-format example: tag overrides the schema when the runtime
	// validation isn't a uuid.Parse-style call (here, it's just used as a
	// timestamp string downstream).
	ExpiresAt string `json:"expiresAt" apispec:"format=date-time"`
}

// CopyDocumentResponse is returned on success. None of these fields are
// converter-validated server-side, so we use the apispec tag to pin them.
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

// Copy decodes the request body and validates every flow-inferred field.
func Copy(w http.ResponseWriter, r *http.Request) {
	var body CopyDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Three uuid.Parse calls on body fields — flow analysis must wire each
	// field back to format: uuid.
	sourceID, err := uuid.Parse(body.SourceID)
	if err != nil {
		http.Error(w, "invalid sourceId", http.StatusBadRequest)
		return
	}
	destID, err := uuid.Parse(body.DestinationBucketID)
	if err != nil {
		http.Error(w, "invalid destinationBucketId", http.StatusBadRequest)
		return
	}
	authID, err := uuid.Parse(body.AuthorizationID)
	if err != nil {
		http.Error(w, "invalid authorizationId", http.StatusBadRequest)
		return
	}
	_, _, _ = sourceID, destID, authID

	// Optional pointer field — skipped if nil; if set, parsed.
	if body.TagsetID != nil {
		if _, err := uuid.Parse(*body.TagsetID); err != nil {
			http.Error(w, "invalid tagsetId", http.StatusBadRequest)
			return
		}
	}

	resp := CopyDocumentResponse{
		ID:         uuid.NewString(),
		CreatedAt:  time.Now().Format(time.RFC3339),
		OwnerEmail: "owner@example.com",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
