//go:build windows

package windows_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/windows"
)

// TestStubsReportOperation runs only on Windows (M6 CI); elsewhere it is
// compiled and vetted by mise run check:windows.
func TestStubsReportOperation(t *testing.T) {
	t.Parallel()

	h := windows.New()
	for op, call := range map[string]func() error{
		"Lstat":         func() error { _, err := h.Lstat(`C:\x`); return err },
		"Stat":          func() error { _, err := h.Stat(`C:\x`); return err },
		"ReadFile":      func() error { _, err := h.ReadFile(`C:\x`); return err },
		"ReadDir":       func() error { _, err := h.ReadDir(`C:\x`); return err },
		"ReadLink":      func() error { _, err := h.ReadLink(`C:\x`); return err },
		"IsManagedLink": func() error { _, err := h.IsManagedLink(`C:\x`, `C:\`); return err },
		"MkdirAll":      func() error { return h.MkdirAll(`C:\x`, 0o755) },
		"WriteAtomic":   func() error { return h.WriteAtomic(`C:\x`, nil, 0o644) },
		"Link":          func() error { return h.Link(`C:\y`, `C:\x`) },
		"Rename":        func() error { return h.Rename(`C:\x`, `C:\y`) },
		"Remove":        func() error { return h.Remove(`C:\x`) },
		"Chmod":         func() error { return h.Chmod(`C:\x`, 0o644) },
		"TryLock":       func() error { _, err := h.TryLock(`C:\x`); return err },
		"HomeDir":       func() error { _, err := h.HomeDir(); return err },
		"Dirs":          func() error { _, err := h.Dirs(); return err },
		"AgentHome":     func() error { _, err := h.AgentHome("CODEX_HOME", ".codex"); return err },
		"Hostname":      func() error { _, err := h.Hostname(); return err },
		"Open":          func() error { return h.Open("https://example.com") },
	} {
		err := call()
		if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), op) {
			t.Errorf("%s error = %v, want ErrUnsupported naming the operation", op, err)
		}
	}
	if _, ok := h.LookupEnv("PATH"); ok || h.IsTerminal(0) {
		t.Error("stub reports environment or terminal, want nothing until M6")
	}
}
