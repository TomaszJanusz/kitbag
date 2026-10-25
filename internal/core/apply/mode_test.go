package apply_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestModePrecedence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, manifest, flag, env, config, platform, want string }{
		{name: "Unix", want: "link"},
		{name: "future Windows", platform: "copy", want: "copy"},
		{name: "config", config: "copy", want: "copy"},
		{name: "environment", env: "link", config: "copy", want: "link"},
		{name: "flag", flag: "link", env: "copy", want: "link"},
		{name: "manifest veto", manifest: "copy", flag: "link", want: "copy"},
		{name: "machine veto", manifest: "link", flag: "copy", want: "copy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := apply.ResolveMode(tc.manifest, tc.flag, tc.env, tc.config, tc.platform)
			must(t, err)
			if got != tc.want {
				t.Fatalf("mode = %s; want %s", got, tc.want)
			}
		})
	}
	if _, err := apply.ResolveMode("", "invalid", "", "", ""); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestManifestHostOverrideAndModeTransition(t *testing.T) {
	t.Parallel()
	h, store := testHost(t)
	d := declaration("file")
	write(t, h, d.SourcePath, "desired")
	m := &manifest.Manifest{Schema: 1, Mode: "link", Hosts: map[string]manifest.Host{"fake-host": {Mode: "copy"}}}
	resolved, err := m.ResolveHost(h, "")
	must(t, err)
	req := request(d)
	req.Manifest = resolved
	assertCode(t, run(t, h, store, req, options("link")), apply.OK)
	if kind := database(t, store).Targets[0].Kind; kind != "copy" {
		t.Fatalf("host mode = %s", kind)
	}
	// An explicit machine copy still wins after a manifest change back to link.
	req.Manifest.Mode = "link"
	assertCode(t, run(t, h, store, req, options("copy")), apply.OK)
	assertCode(t, run(t, h, store, req, options("link")), apply.OK)
	if kind := database(t, store).Targets[0].Kind; kind != "link" {
		t.Fatalf("mode transition = %s", kind)
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want apply.Code
	}{
		{nil, apply.OK},
		{errors.New("I/O"), apply.Warnings},
		{fs.ErrInvalid, apply.Usage},
		{&manifest.VersionError{Required: "1.0.0", Current: "0.1.0"}, apply.ToolTooOld},
		{&platform.UnsupportedOSError{Message: "unsupported"}, apply.UnsupportedOS},
	} {
		if got := apply.ErrorCode(tc.err); got != tc.want {
			t.Fatalf("%v = %d; want %d", tc.err, got, tc.want)
		}
	}
	const expected = "0 1 2 3 4 5"
	if got := fmt.Sprint(apply.OK, " ", apply.Warnings, " ", apply.Usage, " ", apply.Conflict, " ", apply.ToolTooOld, " ", apply.UnsupportedOS); got != expected {
		t.Fatal(got)
	}
}
