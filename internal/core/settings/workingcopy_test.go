package settings

import (
	"path/filepath"
	"testing"
)

func TestChangeWorkingCopyReportsUnusedClone(t *testing.T) {
	t.Parallel()
	s, host := fixture(t)
	home := filepath.Join(string(filepath.Separator), "home", "test")
	must(t, s.Change("repo.url", "https://example.com/agent-config.git", false))
	clone := s.workingCopy()
	if clone == "" || filepath.Base(clone) != "agent-config" {
		t.Fatalf("default working copy = %q", clone)
	}
	// Nothing to report while the default clone does not exist.
	unused, err := s.ChangeWorkingCopy("repo.path", "~/src/agent-config", false)
	must(t, err)
	if unused != "" {
		t.Fatalf("missing clone reported: %q", unused)
	}
	must(t, s.Change("repo.path", "", true))
	must(t, host.MkdirAll(clone, 0o700))
	for _, step := range []struct{ key, value, want string }{
		{"sync.mode", "copy", ""},
		{"repo.path", "~/src/agent-config", clone},
		{"repo.path", "~/src/agent-config", ""},
	} {
		unused, err = s.ChangeWorkingCopy(step.key, step.value, false)
		must(t, err)
		if unused != step.want {
			t.Fatalf("%s=%s reported %q; want %q", step.key, step.value, unused, step.want)
		}
	}
	// Moving back reports the old clone again once it exists; nothing is removed.
	must(t, host.MkdirAll(filepath.Join(home, "src", "agent-config"), 0o700))
	unused, err = s.ChangeWorkingCopy("repo.path", "", true)
	must(t, err)
	if unused != filepath.Join(home, "src", "agent-config") {
		t.Fatalf("unset reported %q", unused)
	}
	if _, err = host.Stat(clone); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeWorkingCopy("repo.path", "relative", false); err == nil {
		t.Fatal("invalid value accepted")
	}
}
