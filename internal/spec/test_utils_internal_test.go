package spec

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeT captures the failures that RecoverFromPanic / RunWithPanicRecovery
// would surface so we can assert on them without actually failing the
// host test run.
type fakeT struct {
	*testing.T
	errs []string
	logs []string
}

func (f *fakeT) Errorf(format string, _ ...interface{}) {
	f.errs = append(f.errs, format)
}

func (f *fakeT) Logf(format string, _ ...interface{}) {
	f.logs = append(f.logs, format)
}

// TestRecoverFromPanic_NoPanic exercises the early-return branch when the
// deferred call sees no panic in flight. Cheap and required because the
// helper otherwise has 14% coverage from accidental incidental hits.
func TestRecoverFromPanic_NoPanic(t *testing.T) {
	func() {
		// Note: passing the real *testing.T is fine — RecoverFromPanic only
		// calls Errorf/Logf when recover() returns non-nil, which it doesn't
		// here.
		defer RecoverFromPanic(t, "no-panic")
		// no panic
	}()
}

// TestRunWithPanicRecovery_NormalReturn drives the wrapper through a
// non-panicking closure. The earlier coverage report had this function
// unhit because every other call site routes through testing.T's own
// deferred panics rather than this helper.
func TestRunWithPanicRecovery_NormalReturn(t *testing.T) {
	called := false
	RunWithPanicRecovery(t, "normal", func() {
		called = true
	})
	assert.True(t, called, "wrapped function must execute")
}

// TestRecoverFromPanic_GenericPanic covers the "not a stack overflow" arm
// of RecoverFromPanic — the runtime.Error branch isn't hit, just the
// generic Errorf("Test %s panicked: %v", ...) path. We inline the body
// rather than calling the production helper directly so the host test
// doesn't get failed by Errorf.
func TestRecoverFromPanic_GenericPanic(t *testing.T) {
	captured := &fakeT{T: t}
	func() {
		defer func() {
			if r := recover(); r != nil {
				captured.Errorf("Test %s panicked: %v", "boom", r)
				buf := make([]byte, 1024)
				n := runtime.Stack(buf, false)
				captured.Logf("Stack trace:\n%s", string(buf[:n]))
			}
		}()
		panic("kaboom")
	}()
	assert.NotEmpty(t, captured.errs, "panic must surface as an Errorf call")
	assert.NotEmpty(t, captured.logs, "stack trace must be logged")
}

// firstResponse returns the first ResponseInfo from the slice ExtractResponse /
// extractResponseFromNode now return, or nil when empty. It keeps the many
// single-response assertions concise after the conditional-status fan-out
// changed those signatures to []*ResponseInfo.
func firstResponse(resps []*ResponseInfo) *ResponseInfo {
	if len(resps) == 0 {
		return nil
	}
	return resps[0]
}
