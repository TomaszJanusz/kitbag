package discover

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// probeRecorder permits exactly one HOME listing and records only Lstat paths.
// All content reads, nested listings and link following still fail via recorder.
type probeRecorder struct {
	recorder
	paths    []string
	listings int
}

func (r *probeRecorder) ReadDir(path string) ([]fs.DirEntry, error) {
	r.listings++
	if path != home() || r.listings != 1 {
		r.t.Errorf("unexpected listing %s (%d)", path, r.listings)
	}
	return r.Host.ReadDir(path)
}

func (r *probeRecorder) Lstat(path string) (fs.FileInfo, error) {
	r.paths = append(r.paths, path)
	return r.Host.Lstat(path)
}

func listTable() Table {
	result := table()
	result[0].ProbePrefix = ".claude"
	result[1].ProbePrefix = ".gemini"
	return result
}

func TestListProbePrivacy(t *testing.T) {
	t.Parallel()
	host := newHost(nil)
	for _, name := range []string{".claude", ".claude_2", ".gemini-work", "unrelated", ".claudette_2", ".claude_"} {
		must(t, host.MkdirAll(filepath.Join(home(), name), dirPerm))
	}
	must(t, host.WriteAtomic(filepath.Join(home(), ".claude_2", ".credentials.json"), []byte("secret"), filePerm))
	must(t, host.WriteAtomic(filepath.Join(home(), "auth.json"), []byte("secret"), filePerm))
	must(t, host.Link(filepath.Join(home(), "unrelated"), filepath.Join(home(), ".claude.link")))
	r := &probeRecorder{recorder: recorder{host, t}}
	got, err := listTable().List(r, nil, "test", true)
	must(t, err)
	want := []string{filepath.Join(home(), ".claude"), filepath.Join(home(), ".gemini"), filepath.Join(home(), ".config", "opencode"), filepath.Join(home(), ".claude.link"), filepath.Join(home(), ".claude_2"), filepath.Join(home(), ".gemini-work")}
	if r.listings != 1 || !slices.Equal(r.paths, want) {
		t.Fatalf("privacy: listings=%d paths=%v", r.listings, r.paths)
	}
	if len(got.Candidates) != 6 || got.Candidates[3].Presence != Symlink || got.Candidates[4].Source != Probe {
		t.Fatalf("probe candidates: %+v", got.Candidates)
	}
}

func TestListNoProbeAndOutsideHome(t *testing.T) {
	t.Parallel()
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": filepath.FromSlash("/srv/claude")})
	got, err := listTable().List(recorder{host, t}, nil, "test", false)
	must(t, err)
	if len(got.Candidates) != 3 || got.Candidates[0].Presence != OutsideHome || got.Candidates[0].Variable != "CLAUDE_CONFIG_DIR" {
		t.Fatalf("candidates: %+v", got)
	}
	if !slices.Equal(got.Kinds, []string{"claude", "gemini", "opencode"}) || got.Instances == nil || got.Warnings == nil {
		t.Fatalf("snapshot: %+v", got)
	}
}

func TestListDeclarationsAndHostReplacement(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte(`schema=1
[targets.claude]
agent="claude"
settings="settings.json"
[targets.claude.instances.main]
dir="~/.claude"
[targets.claude.instances.missing]
dir="~/.claude_missing"
enabled=false
[hosts.mac.targets.claude.instances.second]
dir="~/.claude_2"
settings=false
[targets.custom]
agent="generic"
[targets.custom.instances.only]
dir="~/custom"
roots={instructions="~/custom/AGENTS.md"}
`))
	must(t, err)
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home(), "override"), "KITBAG_HOST": "ignored"})
	must(t, host.MkdirAll(filepath.Join(home(), ".claude_2"), dirPerm))
	got, err := listTable().List(recorder{host, t}, m, "mac", false)
	must(t, err)
	if len(got.Instances) != 4 {
		t.Fatalf("snapshot: %+v", got)
	}
	main, missing, second := got.Instances[0], got.Instances[1], got.Instances[2]
	assertDeclaredDirectories(t, main, second)
	if main.ActiveHere || !slices.Equal(main.ExcludedHosts, []string{"mac"}) {
		t.Fatalf("main: %+v", main)
	}
	if missing.Enabled || missing.Presence != Missing || second.Settings || !second.ActiveHere {
		t.Fatalf("missing/second: %+v %+v", missing, second)
	}
	if len(got.Candidates) != 3 || got.Candidates[0].Dir != filepath.Join(home(), "override") {
		t.Fatalf("candidates: %+v", got.Candidates)
	}
}

func TestListImplicitAndDeduplication(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte("schema=1\n[targets.claude]\nagent='claude'\n"))
	must(t, err)
	got, err := listTable().List(recorder{newHost(nil), t}, m, "test", false)
	must(t, err)
	if len(got.Instances) != 1 || !got.Instances[0].Implicit || !got.Instances[0].ActiveHere || got.Instances[0].Source != Default || len(got.Candidates) != 2 {
		t.Fatalf("snapshot: %+v", got)
	}
}

type listFailure struct {
	platform.Host
	homeErr, readErr error
}

func (h listFailure) HomeDir() (string, error) {
	if h.homeErr != nil {
		return "", h.homeErr
	}
	return h.Host.HomeDir()
}
func (h listFailure) ReadDir(string) ([]fs.DirEntry, error) { return nil, h.readErr }

func TestListErrors(t *testing.T) {
	t.Parallel()
	for _, h := range []ListHost{listFailure{Host: newHost(nil), homeErr: fs.ErrPermission}, listFailure{Host: newHost(nil), readErr: fs.ErrPermission}, newHost(map[string]string{"CLAUDE_CONFIG_DIR": "relative"})} {
		if _, err := listTable().List(h, nil, "test", true); err == nil {
			t.Fatal("ignored discovery failure")
		}
	}
	got, err := listTable().List(lstatError{newHost(nil), &fs.PathError{Op: "lstat", Path: "sensitive", Err: fs.ErrPermission}}, nil, "test", false)
	must(t, err)
	if got.Candidates[0].ErrorClass != "permission" || got.Candidates[0].Presence != Unreadable {
		t.Fatalf("error class: %+v", got)
	}
}

func TestProbeNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".claude_2", ".claude-work", ".claude.alt"} {
		if !probeName(name, ".claude") {
			t.Errorf("rejected %s", name)
		}
	}
	for _, name := range []string{".claude", ".claude_", ".claudette_2", "auth.json", ".claude_/nested", ".claude_\\nested"} {
		if probeName(name, ".claude") {
			t.Errorf("accepted %s", name)
		}
	}
	if probeName("anything", "") {
		t.Fatal("empty prefix matched")
	}
}

func TestListDoesNotMutateManifest(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse("kitbag.toml", []byte("schema=1\n[targets.claude]\nagent='claude'\n[hosts.mac.targets.claude]\nenabled=false\n"))
	must(t, err)
	before := m.Targets["claude"]
	got, err := listTable().List(recorder{newHost(nil), t}, m, "mac", false)
	must(t, err)
	if got.Instances[0].Enabled || got.Instances[0].ActiveHere {
		t.Fatalf("host disable ignored: %+v", got.Instances[0])
	}
	if !reflect.DeepEqual(before, m.Targets["claude"]) {
		t.Fatal("mutated manifest")
	}
}

func assertDeclaredDirectories(t *testing.T, main, second Instance) {
	t.Helper()
	if main.Dir != filepath.Join(home(), ".claude") || main.Source != Declared || !main.Settings {
		t.Fatalf("main directory: %+v", main)
	}
	if second.Presence != Present || !slices.Equal(second.Hosts, []string{"mac"}) {
		t.Fatalf("second directory: %+v", second)
	}
}
