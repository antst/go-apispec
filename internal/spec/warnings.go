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
	"fmt"
	"io"
	"os"
	"sync"
)

// WarningSink collects non-fatal analysis warnings — e.g. an unresolved
// conditional position or an imprecise helper binding (FR-008/FR-012, spec 009) —
// and flushes them to stderr as `warning: <pos>: <message>`. Per Constitution
// Principle IV diagnostics go to stderr and never affect the exit code or the
// machine-parseable stdout spec. A nil *WarningSink is a no-op, so callers need
// not nil-check.
type WarningSink struct {
	mu       sync.Mutex
	out      io.Writer
	warnings []string
}

// NewWarningSink returns a sink that writes warnings to stderr.
func NewWarningSink() *WarningSink {
	return &WarningSink{out: os.Stderr}
}

// Warn records a warning for the given source position and writes it to the sink's
// output. Safe to call on a nil sink (no-op).
func (s *WarningSink) Warn(pos, message string) {
	if s == nil {
		return
	}
	line := fmt.Sprintf("warning: %s: %s", pos, message)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, line)
	if s.out != nil {
		_, _ = fmt.Fprintln(s.out, line)
	}
}

// Warnings returns a copy of the collected warning lines. Safe on a nil sink.
func (s *WarningSink) Warnings() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}
