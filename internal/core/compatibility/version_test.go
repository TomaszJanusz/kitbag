package compatibility

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		manifest string
		required string
		schema   int64
		blocked  bool
	}{
		{"schema=1\nmin_tool_version='9.0.0'", "9.0.0", 0, true},
		{"schema=1", "", 0, false},
		{"invalid", "", 0, false},
		{"schema=1\nmin_tool_version='9.0.0'\nfuture=true", "9.0.0", 0, true},
		{"schema=2\n[future]\nkey=1", "", 2, true},
		{"schema=1\nfuture=true", "", 0, false},
	} {
		t.Run(tc.manifest, func(t *testing.T) {
			t.Parallel()
			host := fake.New()
			repo := filepath.FromSlash("/repo")
			if err := host.MkdirAll(repo, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := host.WriteAtomic(filepath.Join(repo, "kitbag.toml"), []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			var events []Event
			err := Check(host, repo, "1.0.0", func(event Event) { events = append(events, event) })
			if (err != nil) != tc.blocked || (len(events) == 1) != tc.blocked {
				t.Fatalf("error %v, events %+v", err, events)
			}
			if tc.blocked && (events[0] != Event{Type: "tool-too-old", Required: tc.required, Current: "1.0.0", Repository: repo, Schema: tc.schema, SupportedSchema: min(tc.schema, manifest.SchemaLatest)}) {
				t.Fatal(events)
			}
			if tc.blocked && Check(host, repo, "1.0.0", nil) == nil {
				t.Fatal("nil emitter bypassed guard")
			}
		})
	}
}

func TestEventClassification(t *testing.T) {
	t.Parallel()
	if FromError(fs.ErrPermission, "repo") != nil {
		t.Fatal("classified unrelated error")
	}
	event := FromError(errors.Join(fs.ErrInvalid, &manifest.VersionError{Required: "2.0.0", Current: "1.0.0"}), "repo")
	if event == nil || event.Current != "1.0.0" || event.Repository != "repo" {
		t.Fatalf("event %+v", event)
	}
	if err := Check(fake.New(), filepath.FromSlash("/missing"), "1.0.0", nil); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, manifest string
		event          bool
	}{
		{"current", "schema=1\nmin_tool_version='0.1.0'", false},
		{"minimum before validation", "schema=1\nmin_tool_version='9.0.0'\nfuture=true", true},
		{"valid with minimum", "schema=1\nmin_tool_version='9.0.0'", true},
		{"newer schema", "schema=2\nfuture=true", true},
		{"invalid", "schema=1\nfuture=true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := fake.New()
			repo := filepath.FromSlash("/repo")
			if err := host.MkdirAll(repo, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := host.WriteAtomic(filepath.Join(repo, "kitbag.toml"), []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			m, err := Load(host, repo, "1.0.0")
			if event := FromError(err, repo); (event != nil) != tc.event || (event != nil && event.Current != "1.0.0") {
				t.Fatalf("Load error %v, event %+v", err, event)
			}
			if (err == nil) != (m != nil && tc.name == "current") {
				t.Fatalf("Load = %v, %v", m, err)
			}
		})
	}
	if _, err := Load(fake.New(), filepath.FromSlash("/missing"), "1.0.0"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
