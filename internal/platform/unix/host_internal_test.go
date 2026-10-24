//go:build darwin || linux

package unix

import (
	"errors"
	"os"
	"runtime"
	"slices"
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

// TestIsTerminalOnPseudoTerminal opens a pseudo-terminal master, which
// answers terminal attribute requests on Linux and macOS.
func TestIsTerminalOnPseudoTerminal(t *testing.T) {
	t.Parallel()
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	t.Cleanup(func() {
		if err := ptmx.Close(); err != nil {
			t.Error(err)
		}
	})
	h := New(WithEnv(noEnv))
	if !h.IsTerminal(ptmx.Fd()) {
		t.Error("IsTerminal(pty master) = false, want true")
	}
	if h.IsTerminal(^uintptr(0)) {
		t.Error("IsTerminal(out-of-range fd) = true, want false")
	}
}
