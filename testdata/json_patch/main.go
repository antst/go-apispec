// Package main exercises struct-level OpenAPI validation: minProperties and
// anyOf via the `_ struct{} `apispec:"..."“ blank-marker-field idiom.
//
// PATCH endpoints typically reject empty bodies — the handler must receive
// at least one field to know what to update — but the OpenAPI spec usually
// describes the request body as having all-optional fields, leaving the
// "must update at least one of these" invariant invisible to client SDKs.
//
// The marker field below makes that invariant part of the spec so generated
// clients refuse to construct empty PATCH requests and humans reading the
// spec understand the contract.
package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// UpdateDocumentRequest mirrors the alkem-io/file-service-go PATCH body. All
// three fields are optional individually, but at least one must be present —
// expressed via minProperties=1 plus an explicit anyOf listing the eligible
// fields. Both annotations are emitted; clients that only honour one of the
// two still get the constraint enforced.
//
// Fields are pointer-typed so "absent in JSON" (nil) is distinguishable from
// "present but empty" (non-nil pointer to ""). That matches OpenAPI anyOf
// semantics, which check property presence, not value emptiness — runtime
// validation below mirrors the same rule.
type UpdateDocumentRequest struct {
	_ struct{} `apispec:"minProperties=1,anyOf=displayName|storageBucketId|temporaryLocation"`

	DisplayName       *string `json:"displayName,omitempty"`
	StorageBucketID   *string `json:"storageBucketId,omitempty" apispec:"format=uuid"`
	TemporaryLocation *bool   `json:"temporaryLocation,omitempty"`
}

// UpdateDocumentResponse is unconstrained — the marker only goes on inputs
// where "at least one field" is the contract.
type UpdateDocumentResponse struct {
	OK bool `json:"ok"`
}

func main() {
	r := chi.NewRouter()
	r.Patch("/documents/{id}", Update)
	http.ListenAndServe(":3000", r)
}

// Update applies a partial update to a document. Empty bodies are rejected.
func Update(w http.ResponseWriter, r *http.Request) {
	var body UpdateDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := validateAtLeastOne(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UpdateDocumentResponse{OK: true})
}

// validateAtLeastOne mirrors the OpenAPI anyOf constraint at runtime so the
// fixture is self-consistent: handler enforces it, spec advertises it.
// Checks JSON property presence (nil pointer == absent) — empty-but-present
// values like `{"displayName":""}` satisfy the constraint, matching anyOf.
func validateAtLeastOne(body UpdateDocumentRequest) error {
	if body.DisplayName == nil && body.StorageBucketID == nil && body.TemporaryLocation == nil {
		return errors.New("at least one of displayName, storageBucketId, temporaryLocation must be set")
	}
	return nil
}
