// Package main mirrors the alkem-io/file-service DocumentHandler.ReplaceContent
// handler shape locally. This is the bug-reproduction guarantee fixture for
// antst/go-apispec#30: a 7-status switch where the same status (422) is
// written both by a parameter-status helper and by a struct-method Render
// that hardcodes its status.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type errorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

type ReplaceContentResponse struct {
	ExternalID  string `json:"externalID"`
	MimeType    string `json:"mimeType"`
	Size        int    `json:"size"`
	ImageWidth  *int   `json:"imageWidth,omitempty"`
	ImageHeight *int   `json:"imageHeight,omitempty"`
}

func (r ReplaceContentResponse) Render(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(r)
}

// MimeMismatchDetail is the nested-pointer field on RejectedContentResponse
// — present in the original file-service code, replicated here so the
// generated schema matches the report.
type MimeMismatchDetail struct {
	KnownMime    string `json:"knownMime"`
	DetectedMime string `json:"detectedMime"`
}

type RejectedContentResponse struct {
	Code   string              `json:"code"`
	Error  string              `json:"error"`
	Detail *MimeMismatchDetail `json:"detail,omitempty"`
}

func (r RejectedContentResponse) Render(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(r)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg, Code: code})
}

// MimeMismatchError is the typed error caught via errors.As — mirrors the
// service-layer type from the original handler.
type MimeMismatchError struct {
	Known    string
	Detected string
}

func (e *MimeMismatchError) Error() string { return "mime mismatch" }

// MaxBytesError stubs http.MaxBytesError so errors.As works the same way
// as in the original code.
type MaxBytesError struct{}

func (e *MaxBytesError) Error() string { return "request too large" }

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrEmptyContent     = errors.New("empty content")
	ErrImageProcessing  = errors.New("image processing failed")
	ErrConflict         = errors.New("conflict")
)

// DocumentHandler matches the file-service receiver shape.
type DocumentHandler struct{}

// ReplaceContent: PUT /documents/{id}/content — the full file-service shape.
func (h *DocumentHandler) ReplaceContent(w http.ResponseWriter, r *http.Request) {
	docID, err := parseDocID(r)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "invalid document ID")
		return
	}
	_ = docID

	content, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	result, serr := h.storeAndLink(r.Context(), "doc-id", content)
	if serr != nil {
		var mismatch *MimeMismatchError
		switch {
		case errors.Is(serr, ErrDocumentNotFound):
			writeJSONError(w, http.StatusNotFound, "document not found")
		case errors.Is(serr, ErrEmptyContent):
			RejectedContentResponse{Code: "EMPTY_CONTENT", Error: serr.Error()}.Render(w)
		case errors.As(serr, &mismatch):
			RejectedContentResponse{
				Code:  "MIME_MISMATCH",
				Error: mismatch.Error(),
				Detail: &MimeMismatchDetail{
					KnownMime:    mismatch.Known,
					DetectedMime: mismatch.Detected,
				},
			}.Render(w)
		case errors.Is(serr, ErrImageProcessing):
			writeJSONError(w, http.StatusUnprocessableEntity, serr.Error())
		case errors.Is(serr, ErrConflict):
			writeJSONError(w, http.StatusConflict, "conflict with existing document")
		default:
			writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	result.Render(w)
}

func (h *DocumentHandler) storeAndLink(_ context.Context, _ string, _ []byte) (ReplaceContentResponse, error) {
	return ReplaceContentResponse{}, nil
}

func parseDocID(_ *http.Request) (string, error) { return "", nil }

func main() {
	r := chi.NewRouter()
	h := &DocumentHandler{}
	r.Put("/documents/{id}/content", h.ReplaceContent)
	_ = http.ListenAndServe(":3000", r)
}
