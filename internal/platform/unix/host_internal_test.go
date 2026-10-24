//go:build darwin || linux

package unix

import (
	"errors"
	"os"
	"runtime"
	"slices"
	"syscall"
	"testing"
)

func TestOpenRunsDesktopOpener(t *testing.T) {
	t.Parallel()
	var calls [][]string
	h := New(WithEnv(noEnv), WithCommandRunner(func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}))
	if err := h.Open("https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := h.Open("-a"); err == nil {
		t.Error("Open(-a) succeeded, want a validation error")
	}
	want := map[string]string{"darwin": "open", "linux": "xdg-open"}[runtime.GOOS]
	if len(calls) != 1 || !slices.Equal(calls[0], []string{want, "https://example.com"}) {
		t.Errorf("calls = %q, want one %s https://example.com", calls, want)
	}
}

func TestOpenReportsOpenerFailure(t *testing.T) {
	t.Parallel()
	h := New(WithEnv(noEnv), WithCommandRunner(func(string, ...string) error { return errInjected }))
	if err := h.Open("https://example.com"); !errors.Is(err, errInjected) {
		t.Errorf("Open error = %v, want the opener's failure", err)
	}
}

func TestNewUsesProcess(t *testing.T) {
	t.Parallel()
	h := New()
	gotValue, gotOK := h.LookupEnv("PATH")
	wantValue, wantOK := os.LookupEnv("PATH")
	if gotValue != wantValue || gotOK != wantOK {
		t.Error("New().LookupEnv(PATH) differs from the process environment")
	}
	gotName, err := h.Hostname()
	wantName, wantErr := os.Hostname()
	if gotName != wantName || (err == nil) != (wantErr == nil) {
		t.Errorf("Hostname() = %q, %v; want %q, %v", gotName, err, wantName, wantErr)
	}
}

// TestIsTerminalOnPseudoTerminal checks a positive answer on a real
// pseudo-terminal, opened per OS by openTerminal.
func TestIsTerminalOnPseudoTerminal(t *testing.T) {
	t.Parallel()
	h := New(WithEnv(noEnv))
	if term := openTerminal(t); !h.IsTerminal(term.Fd()) {
		t.Errorf("IsTerminal(%s) = false, want true", term.Name())
	}
	if h.IsTerminal(^uintptr(0)) {
		t.Error("IsTerminal(out-of-range fd) = true, want false")
	}
}

// openFile opens a device read-write without making it the controlling
// terminal, skipping the test when pseudo-terminals are unavailable.
func openFile(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0) //nolint:gosec // G304: fixed pseudo-terminal device paths.
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}
