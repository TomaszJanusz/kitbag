package settings

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func fixture(t *testing.T) (Service, *fake.Host) {
	t.Helper()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home, "KITBAG_MODE": "copy"}))
	s := Service{Host: host, Filename: filepath.Join(home, "config.toml"), Flags: map[string]string{}}
	return s, host
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func seed(t *testing.T, s Service) {
	t.Helper()
	must(t, s.Change("repo.url", "local", false))
}

func TestChangesAndSources(t *testing.T) {
	t.Parallel()
	s, host := fixture(t)
	seed(t, s)
	must(t, s.Change("sync.mode", "link", false))
	entry, err := s.Get("sync.mode")
	must(t, err)
	if entry.Value != "copy" || entry.Source != "KITBAG_MODE" {
		t.Fatalf("environment: %+v", entry)
	}
	s.Flags["mode"] = "link"
	entry, err = s.Get("sync.mode")
	must(t, err)
	if entry.Value != "link" || entry.Source != "--mode" {
		t.Fatalf("flag: %+v", entry)
	}
	must(t, s.Change("sync.mode", "", true))
	data, values, err := s.Read()
	must(t, err)
	if _, ok := values["sync.mode"]; ok {
		t.Fatalf("unset left residue: %s", data)
	}
	info, err := host.Lstat(s.Filename)
	must(t, err)
	if info.Mode().Perm() != filePerm {
		t.Fatalf("mode: %v", info.Mode())
	}
	entries, err := s.List()
	must(t, err)
	if len(entries) == 0 {
		t.Fatal("empty list")
	}
	sensitive, err := s.Get("repo.url")
	must(t, err)
	if sensitive.Display() != "[redacted]" {
		t.Fatal(sensitive.Display())
	}
}

func TestRejectedChangesPreserveFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key, text string
		unset     bool
	}{
		{key: "sync.mode", text: "bad"},
		{key: "ui.notifications", text: "2"},
		{key: "schema", text: "2"},
		{key: "repo.path", text: "/absolute"},
		{key: "git.identity.name", text: "unpaired"},
		{key: "migration.legacy_roots", text: `["~/x", "~/x"]`},
		{key: "repo.url", unset: true},
		{key: "schema", unset: true},
		{key: "unknown", text: "value"},
	} {
		t.Run(tc.key+tc.text, func(t *testing.T) {
			t.Parallel()
			s, _ := fixture(t)
			seed(t, s)
			before, _, err := s.Read()
			must(t, err)
			if err = s.Change(tc.key, tc.text, tc.unset); err == nil {
				t.Fatal("invalid change accepted")
			}
			after, _, err := s.Read()
			must(t, err)
			if string(before) != string(after) {
				t.Fatal("invalid write clobbered config")
			}
		})
	}
}

func TestEditOutcomes(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"valid", "abort", "retry", "runner", "callback", "concurrent"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			s, host := fixture(t)
			seed(t, s)
			original, _, err := s.Read()
			must(t, err)
			attempts := 0
			editor := func(path string) error {
				attempts++
				return editOutcome(host, s, path, outcome, attempts)
			}
			retry := func(error) (bool, error) {
				if outcome == "callback" {
					return false, fs.ErrPermission
				}
				return outcome == "retry", nil
			}
			err = s.Edit(editor, retry)
			wantError := outcome == "runner" || outcome == "callback" || outcome == "concurrent"
			if (err != nil) != wantError {
				t.Fatalf("error: %v", err)
			}
			verifyEdit(t, s, original, outcome)
			if _, err = host.Lstat(s.Filename + ".edit.toml"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("draft leaked: %v", err)
			}
		})
	}
}

func editOutcome(host *fake.Host, s Service, path, outcome string, attempts int) error {
	if outcome == "runner" {
		return fs.ErrPermission
	}
	if outcome == "concurrent" {
		if err := host.WriteAtomic(s.Filename, []byte("external"), filePerm); err != nil {
			return err
		}
	}
	data := "schema=1\n[repo]\nurl='local'\n[sync]\nmode='copy'\n"
	if outcome == "abort" || outcome == "callback" || (outcome == "retry" && attempts == 1) {
		data += "unknown=true\n"
	}
	return host.WriteAtomic(path, []byte(data), filePerm)
}

func verifyEdit(t *testing.T, s Service, original []byte, outcome string) {
	t.Helper()
	after, err := s.Host.ReadFile(s.Filename)
	must(t, err)
	switch outcome {
	case "valid", "retry":
		if !strings.Contains(string(after), "mode='copy'") {
			t.Fatalf("saved: %s", after)
		}
	case "concurrent":
		if string(after) != "external" {
			t.Fatal("external edit clobbered")
		}
	default:
		if string(after) != string(original) {
			t.Fatal("original clobbered")
		}
	}
}

func TestWriteGuards(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"comment", "draft", "symlink", "locked", "invalid", "directory"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			s, host := fixture(t)
			seed(t, s)
			prepareGuard(t, s, host, kind)
			err := s.Change("sync.mode", "link", false)
			if kind == "draft" {
				err = s.Edit(func(string) error { t.Fatal("editor ran"); return nil }, func(error) (bool, error) { return false, nil })
			}
			if err == nil {
				t.Fatal("guard accepted unsafe write")
			}
		})
	}
}

func prepareGuard(t *testing.T, s Service, host *fake.Host, kind string) {
	t.Helper()
	switch kind {
	case "comment":
		must(t, host.WriteAtomic(s.Filename, []byte("# comment\nschema=1\n[repo]\nurl='local'"), filePerm))
	case "draft":
		must(t, host.WriteAtomic(s.Filename+".edit.toml", []byte("preserve"), filePerm))
	case "symlink":
		must(t, host.Link("elsewhere", s.Filename))
	case "locked":
		lock, err := host.TryLock(s.Filename + ".settings.lock")
		must(t, err)
		t.Cleanup(func() { must(t, lock.Unlock()) })
	case "invalid":
		must(t, host.WriteAtomic(s.Filename, []byte("unknown=true"), filePerm))
	case "directory":
		must(t, host.Remove(s.Filename))
		must(t, host.MkdirAll(s.Filename, directoryPerm))
	}
}

func TestReadErrorsAndDisplay(t *testing.T) {
	t.Parallel()
	s, host := fixture(t)
	_, err := s.Get("typo")
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	must(t, host.MkdirAll(s.Filename, directoryPerm))
	if _, err = s.Get("host"); err == nil {
		t.Fatal("directory read accepted")
	}
	if _, err = s.List(); err == nil {
		t.Fatal("list swallowed error")
	}
	if got := (Entry{Value: "a\nb"}).Display(); got != `a\nb` {
		t.Fatal(got)
	}
}
