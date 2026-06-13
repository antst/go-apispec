// Package main guards issue #35: string-literal indexes into a plain
// map[string]string (e.g. multipart form fields buffered into a map) must NOT
// be misinferred as in:path parameters. Only map indexes that correspond to a
// real {placeholder} in the route template are path parameters.
//
// Two routes exercise both sides of the invariant:
//
//   - POST /internal/file has no path placeholders. Its handler reads
//     fields["displayName"] from a plain map inside a helper. The generated
//     operation must have no path parameters (the #35 false positive).
//   - GET /widgets/{id} reads mux.Vars(r)["id"], which matches the {id}
//     placeholder, so that path parameter must still be emitted (the control —
//     the fix must not over-prune legitimate router-var reads).
package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/internal/file", CreateFile).Methods("POST")
	r.HandleFunc("/widgets/{id}", GetWidget).Methods("GET")
	http.ListenAndServe(":8080", r)
}

// buildInput reads string-literal keys from a plain field map (issue #35). The
// map does not originate from mux.Vars, and the route has no placeholders, so
// none of these keys are path parameters.
func buildInput(fields map[string]string) (string, string) {
	displayName := fields["displayName"]
	bucket := fields["storageBucketId"]
	return displayName, bucket
}

// CreateFile buffers multipart fields into a map and hands them to a helper.
func CreateFile(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	fields := map[string]string{}
	for k, v := range r.MultipartForm.Value {
		if len(v) > 0 {
			fields[k] = v[0]
		}
	}
	name, bucket := buildInput(fields)
	_ = name
	_ = bucket
	w.WriteHeader(http.StatusCreated)
}

// GetWidget reads a genuine router var that matches the {id} placeholder.
func GetWidget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	_ = id
	w.WriteHeader(http.StatusOK)
}
