package settings

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

func TestEntriesResolveBytesLikeList(t *testing.T) {
	t.Parallel()
	s, host := fixture(t)
	s.Flags = map[string]string{"git-backend": "system"}
	seed(t, s)
	must(t, s.Change("ui.theme", "dark", false))
	data, _, err := s.Read()
	must(t, err)
	listed, err := s.List()
	must(t, err)
	resolved, err := Entries(host, data, s.Flags)
	must(t, err)
	if len(resolved) != len(settingskey.Registry()) || len(resolved) != len(listed) {
		t.Fatalf("entries=%d listed=%d", len(resolved), len(listed))
	}
	sources := map[string]string{}
	for i, entry := range resolved {
		sources[entry.Key.Path] = entry.Source
		if entry.Key.Path != listed[i].Key.Path || entry.Value != listed[i].Value || entry.Source != listed[i].Source {
			t.Errorf("%s: bytes=%+v list=%+v", listed[i].Key.Path, entry, listed[i])
		}
	}
	want := map[string]string{"ui.theme": "ui.theme in config.toml", "sync.mode": "KITBAG_MODE", "git.backend": "--git-backend", "ui.color": "default"}
	for path, source := range want {
		if sources[path] != source {
			t.Errorf("%s source=%q want %q", path, sources[path], source)
		}
	}
}

func TestEntriesForAbsentAndInvalidFiles(t *testing.T) {
	t.Parallel()
	_, host := fixture(t)
	empty, err := Entries(host, nil, nil)
	must(t, err)
	if empty[0].Key.Path != "schema" || empty[0].Value != "1" || empty[0].Source != "default" {
		t.Fatalf("absent file=%+v", empty[0])
	}
	if _, err = Entries(host, []byte("schema = 2"), nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid bytes=%v", err)
	}
	if _, err = Entries(host, nil, map[string]string{"mode": "bogus"}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid override=%v", err)
	}
}

func TestChangedIsPureAndValidated(t *testing.T) {
	t.Parallel()
	created, err := Changed(nil, "repo.url", "https://example.invalid/c.git", false)
	must(t, err)
	if !strings.Contains(string(created), "schema = 1") || !strings.Contains(string(created), "url = 'https://example.invalid/c.git'") {
		t.Fatalf("created=%s", created)
	}
	updated, err := Changed(created, "ui.theme", "light", false)
	must(t, err)
	removed, err := Changed(updated, "ui.theme", "", true)
	must(t, err)
	if !strings.Contains(string(updated), "theme = 'light'") || strings.Contains(string(removed), "theme") {
		t.Fatalf("updated=%s removed=%s", updated, removed)
	}
	for _, tc := range []struct {
		name, path, value string
		data              []byte
		unset             bool
	}{
		{"unknown key", "nope", "x", created, false},
		{"invalid value", "ui.theme", "neon", created, false},
		{"required key", "repo.url", "", created, true},
		{"paired identity", "git.identity.name", "Ada", created, false},
		{"comment", "ui.theme", "dark", append([]byte("# keep\n"), created...), false},
		{"invalid file", "ui.theme", "dark", []byte("schema = 2"), false},
	} {
		if _, err = Changed(tc.data, tc.path, tc.value, tc.unset); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}
