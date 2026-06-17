// Package main exercises Go 1.22 ServeMux wildcard patterns: a trailing
// {path...} multi-segment wildcard must normalize to {path}, and the {$}
// end-of-path anchor must be dropped from the OpenAPI path.
package main

import "net/http"

func serveFile(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("path")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

func listRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files/{path...}", serveFile)
	mux.HandleFunc("GET /exact/{$}", listRoot)
	_ = http.ListenAndServe(":8080", mux)
}
