// Package handlers mirrors the alkem-io/file-service shape from issue #41: the
// route handlers are methods on a handler struct, in a package separate from
// where routes are registered, and they share a free-function any-typed decode
// helper. The handler struct is reached at the registration site through a
// dependency-struct field selector inside an r.Route(...) closure, so
// RouteInfo.Function ends up in registration-site form. Per-call-site
// resolution must still bind each endpoint to its own request DTO via the
// (package, method-name) match.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

type CopyDocumentRequest struct {
	SourceID            string `json:"sourceId"`
	DestinationBucketID string `json:"destinationBucketId"`
	AuthorizationID     string `json:"authorizationId"`
}

type UpdateDocumentRequest struct {
	StorageBucketID *string `json:"storageBucketId,omitempty"`
	DisplayName     *string `json:"displayName,omitempty"`
}

// DocumentHandler carries the route handlers as methods.
type DocumentHandler struct{}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

// decodeStrictJSON is the shared, free-function, any-typed decode helper.
func decodeStrictJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if dec.More() {
		writeJSONError(w, http.StatusBadRequest, "trailing data")
		return false
	}
	return true
}

// Copy handles POST /internal/file/copy.
func (h *DocumentHandler) Copy(w http.ResponseWriter, r *http.Request) {
	var body CopyDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	if _, err := uuid.Parse(body.SourceID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid sourceId")
		return
	}
	if _, err := uuid.Parse(body.DestinationBucketID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid destinationBucketId")
		return
	}
	if _, err := uuid.Parse(body.AuthorizationID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid authorizationId")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// Update handles PATCH /internal/file/{id}.
func (h *DocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	var body UpdateDocumentRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}
	if body.StorageBucketID != nil {
		if _, err := uuid.Parse(*body.StorageBucketID); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid storageBucketId")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
