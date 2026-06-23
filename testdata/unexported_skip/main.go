// Package main exercises struct-field visibility and json-tag option handling
// in schema generation, all matching encoding/json's marshaling rules:
//
//   - an unexported field is never marshaled, so it must not appear as a property
//     (FIX 1);
//   - a field tagged exactly `json:"-"` is dropped entirely (FIX 2), while the Go
//     subtlety `json:"-,"` (trailing comma) names a field literally "-" and IS
//     kept (FIX 2);
//   - a numeric/bool field with the `,string` option marshals as a quoted JSON
//     string, so its schema is {type:string} (FIX 4) — and a named-integer enum
//     tagged `,string` carries its enum members as STRINGS, not raw ints (FIX 4);
//   - but `,string` on a SLICE/map is ignored by encoding/json, so a `[]Enum`
//     field keeps a raw integer element enum, never stringified (FIX 4).
//
// Embedded promotion is included so the unexported-leaf skip is exercised through
// field promotion as well as direct fields.
package main

import (
	"encoding/json"
	"net/http"
)

// audit is embedded anonymously; only its EXPORTED field is promoted, and its
// unexported leaf must not leak into the parent schema.
type audit struct {
	CreatedBy string `json:"createdBy"`
	secret    string // unexported leaf of an embedded type — never marshaled
}

// Tier is a named integer enum. Tagged `,string` (below), encoding/json marshals
// each value as a QUOTED string, so the schema must be {type:string} with the
// enum members as STRINGS ("0","1","2") — not the raw integers, which could never
// validate against type:string (FIX 4: the enum must follow the forced type).
type Tier int

const (
	TierFree Tier = 0
	TierPro  Tier = 1
	TierMax  Tier = 2
)

// Account mixes exported, unexported, json:"-", json:"-," and ,string fields.
type Account struct {
	audit

	ID       int     `json:"id"`
	Name     string  `json:"name"`
	password string  // unexported — never marshaled, must be skipped
	internal int     // unexported — never marshaled, must be skipped
	Token    string  `json:"-"`                                // explicit skip — must be omitted
	Dash     string  `json:"-,"`                               // literal field name "-" — must be KEPT
	Balance  int64   `json:",string"`                          // quoted-string number — {type:string}, name "Balance"
	Ratio    float64 `json:"ratio,string"`                     // quoted-string number under explicit json name
	Active   bool    `json:"active,string"`                    // quoted-string bool — {type:string}
	Tier     Tier    `json:"tier,string"`                      // named-int enum + ,string → {type:string, enum:["0","1","2"]}
	Tiers    []Tier  `json:"tiers,string"`                     // ,string is IGNORED on a slice → array of RAW int enum [0,1,2]
	Capped   int     `json:"capped,string" validate:"max=100"` // ,string + numeric validate → clean {type:string}, no maxLength
}

func getAccount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Account{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /account", getAccount)
	_ = http.ListenAndServe(":8080", mux)
}
