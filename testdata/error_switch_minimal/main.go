// Package main is the minimal regression fixture for antst/go-apispec#30 —
// a chi handler whose error switch writes different response schemas under
// different status codes. Before the Layer-1 fix, the schema bound to 422 by
// RejectedContentResponse.Render leaks to 400.
package main

import (
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
	ExternalID string `json:"externalID"`
	Size       int    `json:"size"`
}

func (r ReplaceContentResponse) Render(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(r)
}

type RejectedContentResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
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

var (
	ErrEmptyContent    = errors.New("empty content")
	ErrImageProcessing = errors.New("image processing failed")
)

// Handler is a method receiver — required for the route-in-route arg-node
// bug to fire (the handler-ref arg-node carries the function body subtree,
// where the second-pass Encode re-fires).
type Handler struct{}

// ReplaceContent is the buggy shape: 422 has both a helper-write AND a
// struct-method-write. Pre-fix, RejectedContentResponse leaks to 400.
func (h *Handler) ReplaceContent(w http.ResponseWriter, r *http.Request) {
	content, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	_ = content

	serr := process()
	if serr != nil {
		switch {
		case errors.Is(serr, ErrEmptyContent):
			RejectedContentResponse{Code: "EMPTY_CONTENT", Error: serr.Error()}.Render(w)
		case errors.Is(serr, ErrImageProcessing):
			writeJSONError(w, http.StatusUnprocessableEntity, serr.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, "unknown error")
		}
		return
	}

	ReplaceContentResponse{ExternalID: "ext-1", Size: 0}.Render(w)
}

func process() error { return nil }

func main() {
	r := chi.NewRouter()
	h := &Handler{}
	r.Put("/documents/{id}/content", h.ReplaceContent)
	_ = http.ListenAndServe(":3000", r)
}
