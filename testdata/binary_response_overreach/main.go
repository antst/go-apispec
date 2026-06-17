// Package main guards the response-side destination check (issue #52): a
// free-function write (io.Copy/io.WriteString/fmt.Fprintf) only yields a
// response when its destination is the HTTP ResponseWriter.
//
//   - Download streams a file straight to w via io.Copy(w, f) — a legitimate
//     binary 200 that MUST be preserved.
//   - Upload's call graph reaches io.Copy(dst, src) where dst is an *os.File
//     (store) — that copy must NOT be misinferred as Upload's response body.
package main

import (
	"io"
	"net/http"
	"os"
)

// Handler holds both endpoints.
type Handler struct{}

// Download streams raw bytes to the response writer — a real binary 200.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	f := h.open(r.PathValue("id"))
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, f)
}

// open returns the blob file for an id.
func (h *Handler) open(id string) *os.File {
	f, _ := os.Open("/data/" + id)
	return f
}

// Upload is a multipart upload returning 201; its body is never binary.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	h.store(file)
	w.WriteHeader(http.StatusCreated)
}

// store copies the upload to a file on disk — io.Copy whose destination is an
// *os.File, not the response writer.
func (h *Handler) store(src io.Reader) {
	dst, err := os.Create("/tmp/blob")
	if err != nil {
		return
	}
	defer dst.Close()
	_, _ = io.Copy(dst, src)
}

func main() {
	h := &Handler{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /file/{id}", h.Download)
	mux.HandleFunc("POST /file", h.Upload)
	_ = http.ListenAndServe(":8080", mux)
}
