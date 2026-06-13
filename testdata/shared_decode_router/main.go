// Package main wires the handlers through a dependency struct and an
// r.Route(...) closure, the registration shape that left issue #41 broken
// after v0.4.20 (handler reached via deps.DocumentHandler.Copy inside the
// closure rather than a direct h.Copy).
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"shared_decode_router/handlers"
)

type deps struct {
	DocumentHandler *handlers.DocumentHandler
}

func main() {
	d := &deps{DocumentHandler: &handlers.DocumentHandler{}}
	r := chi.NewRouter()
	r.Route("/internal", func(r chi.Router) {
		r.Post("/file/copy", d.DocumentHandler.Copy)
		r.Patch("/file/{id}", d.DocumentHandler.Update)
	})
	http.ListenAndServe(":3000", r)
}
