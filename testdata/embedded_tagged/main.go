// Package main exercises encoding/json's tag-dependent rules for ANONYMOUS
// embedded struct fields, which the analyzer must mirror:
//
//   - an UNTAGGED embed (Base) has its exported fields PROMOTED/flattened onto
//     the outer object;
//   - an embed tagged with a NAME (Meta `json:"meta"`) is marshaled as a NESTED
//     object under that name — NOT flattened;
//   - an embed tagged exactly `json:"-"` (Internal) is DROPPED entirely — neither
//     promoted nor nested, so its type never appears as a component;
//   - an UNEXPORTED-type embed with a json name (secretBag `json:"secrets"`) is
//     STILL marshaled nested under that name (the anonymous-field visibility
//     amendment), so it must appear — not be dropped as "unexported";
//   - a normal scalar field (Title) is unaffected.
package main

import (
	"encoding/json"
	"net/http"
)

// Base is embedded WITHOUT a json tag, so its exported fields promote flat onto
// Document (id, createdAt appear at the top level).
type Base struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

// Meta is embedded WITH a json name, so it is marshaled as a nested object under
// "meta" rather than flattened. Its own fields stay inside that object.
type Meta struct {
	Revision int    `json:"revision"`
	Author   string `json:"author"`
}

// Internal is embedded with `json:"-"`, so encoding/json omits it entirely; it
// must not contribute any property and must not be emitted as a component.
type Internal struct {
	Secret string `json:"secret"`
}

// secretBag is an UNEXPORTED struct type embedded WITH a json name. encoding/json
// marshals a json-named anonymous embed regardless of the type's exportedness, so
// it appears nested under "secrets" — the analyzer must not drop it as unexported.
type secretBag struct {
	Token string `json:"token"`
}

// tally is an UNEXPORTED NON-struct type. encoding/json marshals a json-named embed
// of an unexported type ONLY when its underlying is a struct, so an unexported
// non-struct embed is DROPPED even with a tag — the analyzer must not emit it.
type tally int

// Document mixes every embed/field kind.
type Document struct {
	Base                       // untagged embed -> flattened (id, createdAt)
	Meta      `json:"meta"`    // named embed -> nested {"meta": {...}}
	Internal  `json:"-"`       // dropped entirely
	secretBag `json:"secrets"` // unexported struct embed, named -> nested {"secrets": {...}}
	tally     `json:"tally"`   // unexported NON-struct embed, named -> DROPPED by encoding/json
	Title     string           `json:"title"`
}

func getDoc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Document{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /doc", getDoc)
	_ = http.ListenAndServe(":8080", mux)
}
