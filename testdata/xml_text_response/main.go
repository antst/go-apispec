// Package main exercises non-JSON response content types: an application/xml
// response encoded with encoding/xml, and a text/plain response — alongside a
// JSON endpoint, so the generated content maps span more than application/json.
package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
)

// Note is serialised as XML.
type Note struct {
	XMLName xml.Name `xml:"note" json:"-"`
	To      string   `xml:"to" json:"to"`
	From    string   `xml:"from" json:"from"`
	Body    string   `xml:"body" json:"body"`
}

func getNoteXML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(Note{})
}

func getStatusText(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "OK")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notes/{id}", getNoteXML)
	mux.HandleFunc("GET /status", getStatusText)
	_ = http.ListenAndServe(":8080", mux)
}
