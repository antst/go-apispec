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

package spec

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWarningSink(t *testing.T) {
	var buf bytes.Buffer
	s := &WarningSink{out: &buf}
	s.Warn("f.go:10:2", "imprecise binding")
	s.Warn("f.go:20:4", "unresolved status")

	assert.Equal(t, []string{
		"warning: f.go:10:2: imprecise binding",
		"warning: f.go:20:4: unresolved status",
	}, s.Warnings())
	assert.Contains(t, buf.String(), "warning: f.go:10:2: imprecise binding")
	assert.Contains(t, buf.String(), "warning: f.go:20:4: unresolved status")
}

func TestWarningSink_NilSafe(t *testing.T) {
	var nilSink *WarningSink
	nilSink.Warn("x", "y") // must not panic
	assert.Nil(t, nilSink.Warnings())
}

func TestNewWarningSink(t *testing.T) {
	assert.NotNil(t, NewWarningSink())
}
