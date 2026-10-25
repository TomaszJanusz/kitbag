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
		blocked  bool
	}{
		{"schema=1\nmin_tool_version='9.0.0'", true}, {"schema=1", false}, {"invalid", false},
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
			if tc.blocked && events[0].Required != "9.0.0" {
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
