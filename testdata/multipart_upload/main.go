// Package main exercises multipart/form-data file uploads and form-value
// extraction via the Go 1.22 ServeMux, plus a non-JSON (text/plain) response —
// content shapes the JSON-DTO fixtures don't cover.
package main

import (
	"fmt"
	"net/http"
)

// uploadAvatar reads a multipart file plus a form field and replies with plain
// text.
func uploadAvatar(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(10 << 20)
	file, header, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "missing avatar", http.StatusBadRequest)
		return
	}
	defer file.Close()
	caption := r.FormValue("caption")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "uploaded %s (%s)", header.Filename, caption)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users/{id}/avatar", uploadAvatar)
	_ = http.ListenAndServe(":8080", mux)
}
