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

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/antst/go-apispec/internal/engine"
)

func TestWarnSkippedPackages(t *testing.T) {
	// No skipped packages -> no output.
	var empty bytes.Buffer
	warnSkippedPackages(&empty, nil)
	if empty.Len() != 0 {
		t.Errorf("expected no output for empty input, got %q", empty.String())
	}

	// Skipped packages -> header + one line each, with an empty reason falling
	// back to "type error".
	var buf bytes.Buffer
	warnSkippedPackages(&buf, []engine.SkippedPackage{
		{Package: "example.com/app/broken", Reason: "undefined: X"},
		{Package: "example.com/app/empty"},
	})
	out := buf.String()
	for _, want := range []string{
		"2 in-module package(s) skipped",
		"example.com/app/broken: undefined: X",
		"example.com/app/empty: type error",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("warning output missing %q\ngot:\n%s", want, out)
		}
	}
}
