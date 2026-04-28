// Package main demonstrates multipart form handlers that read form values
// through different idioms — inline conversion, var+guard+converter, direct
// string usage after a non-empty check, and r.FormFile parts.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// UploadResponse describes the result of an upload request.
type UploadResponse struct {
	OK bool `json:"ok"`
}

func main() {
	r := chi.NewRouter()
	r.Post("/upload", Upload)
	http.ListenAndServe(":3000", r)
}

// Upload accepts a multipart-form POST and reads several fields, each via a
// different idiom. Every field is part of the public API contract and should
// appear as a form parameter in the generated spec.
func Upload(w http.ResponseWriter, r *http.Request) {
	// Pattern 1: inline + typed converter — currently detected.
	storageBucketID, err := strconv.Atoi(r.FormValue("storageBucketId"))
	if err != nil {
		http.Error(w, "invalid storageBucketId", http.StatusBadRequest)
		return
	}
	_ = storageBucketID

	// Pattern 2: var + guard + ParseBool — should be detected.
	temporaryLocation := false
	if v := r.FormValue("temporaryLocation"); v != "" {
		parsed, perr := strconv.ParseBool(v)
		if perr != nil {
			http.Error(w, "invalid temporaryLocation", http.StatusBadRequest)
			return
		}
		temporaryLocation = parsed
	}
	_ = temporaryLocation

	// Pattern 3: var + guard + Atoi — should be detected.
	maxFileSize := 0
	if v := r.FormValue("maxFileSize"); v != "" {
		parsed, perr := strconv.Atoi(v)
		if perr != nil || parsed < 0 {
			http.Error(w, "invalid maxFileSize", http.StatusBadRequest)
			return
		}
		maxFileSize = parsed
	}
	_ = maxFileSize

	// Pattern 4: direct string usage after non-empty check — should be detected.
	displayName := r.FormValue("displayName")
	if displayName == "" {
		http.Error(w, "displayName is required", http.StatusBadRequest)
		return
	}

	// Pattern 5: r.FormFile multipart part — should be detected as form file.
	file, header, ferr := r.FormFile("file")
	if ferr != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	_ = header

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UploadResponse{OK: true})
	_ = fmt.Sprint(displayName)
}
