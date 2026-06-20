// Copyright 2025 Ehab Terra, 2025-2026 Anton Starikov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import "testing"

// TestParseTypeRef_RoundTrip asserts ParseTypeRef inverts String() for the
// canonical forms (and the equivalent variants the pipeline produces).
func TestParseTypeRef_RoundTrip(t *testing.T) {
	cases := []string{
		"string", "int", "time.Time", "interface{}", "struct{}",
		"*int", "[]string", "[]*User", "[3]int", "[16]byte", "[...]int",
		"map[string]int", "map[string]*Money", "map[string][]Money",
		"User", "models.User", "github.com/gofiber/fiber/v2.Map",
		"APIResponse[models.User]", "Pair[string,int]",
		"Pair[github.com/x/pkg.User,github.com/x/pkg.Order]",
		"[]map[string]*models.User",
		"map[string]Outer[Inner[int],string]",
	}
	for _, s := range cases {
		ref := ParseTypeRef(s)
		if ref == nil {
			t.Errorf("ParseTypeRef(%q) = nil", s)
			continue
		}
		if got := ref.String(); got != s {
			t.Errorf("round-trip: ParseTypeRef(%q).String() = %q", s, got)
		}
	}

	// "any" is interface{}; "-->" normalizes to ".".
	if r := ParseTypeRef("any"); r == nil || r.Kind != RefInterface {
		t.Errorf("any should parse to RefInterface, got %v", r)
	}
	if r := ParseTypeRef("models-->User"); r == nil || r.String() != "models.User" {
		t.Errorf("--> normalize failed: %v", r)
	}
	// Opaque kinds.
	if r := ParseTypeRef("func(int) error"); r == nil || r.Kind != RefFunc {
		t.Errorf("func should parse to RefFunc, got %v", r)
	}
	if r := ParseTypeRef("chan int"); r == nil || r.Kind != RefChan {
		t.Errorf("chan should parse to RefChan, got %v", r)
	}
	// Empty / whitespace → nil.
	if ParseTypeRef("") != nil || ParseTypeRef("  ") != nil {
		t.Error("empty should be nil")
	}

	// NamedLeaf unwraps containers to the named/basic leaf.
	check := func(in, wantLeaf string) {
		if leaf := ParseTypeRef(in).NamedLeaf(); leaf == nil || leaf.String() != wantLeaf {
			t.Errorf("ParseTypeRef(%q).NamedLeaf() = %v, want %q", in, leaf, wantLeaf)
		}
	}
	check("models.User", "models.User")
	check("*models.User", "models.User")
	check("[]models.User", "models.User")
	check("map[string][]*models.User", "models.User")
	check("[3]int", "int")
	if (&TypeRef{Kind: RefStruct}).NamedLeaf().Kind != RefStruct {
		t.Error("NamedLeaf of a non-container should return itself")
	}
	if (*TypeRef)(nil).NamedLeaf() != nil {
		t.Error("NamedLeaf(nil) should be nil")
	}

	// Malformed inputs → nil (the error/guard branches).
	bad := []string{
		"*",          // pointer with no element
		"[]",         // slice with no element
		"[...]",      // inferred array with no element
		"[3]",        // fixed array with no element
		"[x]int",     // non-numeric array length
		"map[string", // unterminated map key
		"map[]int",   // empty map key
		"[]*",        // slice of malformed element
	}
	for _, s := range bad {
		if r := ParseTypeRef(s); r != nil {
			t.Errorf("ParseTypeRef(%q) = %v, want nil", s, r)
		}
	}
}
