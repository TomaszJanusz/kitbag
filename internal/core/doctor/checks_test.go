package doctor

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// testHome is the HOME of [subjectWith]; codexHome is its default Codex home.
func testHome() string  { return filepath.FromSlash("/home/doctor") }
func codexHome() string { return filepath.Join(testHome(), ".codex") }

const codexManifest = "schema=1\n[targets.codex]\nagent=\"codex\"\n"

// expect is one asserted result: status, a message fragment and the fix hint
// fragment, which must be empty for OK results.
type expect struct {
	id, message, fix string
	status           Status
}

// assertChecks verifies expectations, optional exact order and absent fragments.
func assertChecks(t *testing.T, r Report, order []string, expects []expect, absent ...string) {
	t.Helper()
	if order != nil && !slices.Equal(ids(r), order) {
		t.Fatalf("checks = %q, want %q: %+v", ids(r), order, r)
	}
	for _, e := range expects {
		assertCheck(t, find(t, r, e.id), e)
	}
	for _, check := range r.Checks {
		for _, leaked := range absent {
			if strings.Contains(check.Message, leaked) {
				t.Errorf("%s leaked %q: %q", check.ID, leaked, check.Message)
			}
		}
	}
}

func assertCheck(t *testing.T, got Check, e expect) {
	t.Helper()
	if got.Status != e.status || !strings.Contains(got.Message, e.message) {
		t.Errorf("%s = %+v, want %s containing %q", e.id, got, e.status, e.message)
	}
	if e.status == OK && got.Fix != "" || e.status != OK && (e.fix == "" || !strings.Contains(got.Fix, e.fix)) {
		t.Errorf("%s fix = %q, want %q", e.id, got.Fix, e.fix)
	}
}

// ids returns the check identifiers in report order.
func ids(r Report) []string {
	out := make([]string, 0, len(r.Checks))
	for _, check := range r.Checks {
		out = append(out, check.ID)
	}
	return out
}

func TestRegistryOrder(t *testing.T) {
	t.Parallel()
	s, _ := subjectWith(t, nil, codexManifest)
	s.Config.Migration.LegacyRoots = []string{"~/old"}
	r := s.Run(t.Context())
	assertChecks(t, r, []string{
		"tool-version", "instance-codex/default",
		"git-backend", "remote", "commit-identity", "commit-signing",
		"ssh-agent", "agent-claude", "agent-codex",
		"state-lock", "state-health", "materialization-mode", "copy", "link",
		"legacy-links", "legacy-markers", "legacy-settings-markers", "legacy-units",
	}, nil)
	if r.DoctorSchema != 1 {
		t.Fatal("schema", r.DoctorSchema)
	}
	for _, check := range r.Checks {
		if check.Status != OK && check.Fix == "" {
			t.Errorf("%s is %s without a fix", check.ID, check.Status)
		}
	}
}

func TestFailedPrerequisiteContinues(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	must(t, host.Remove(filepath.Join(s.Repository, "kitbag.toml")))
	r := s.Run(t.Context())
	assertChecks(t, r, nil, []expect{
		{"manifest", "cannot load manifest/host", "repair kitbag.toml", Fail},
		{"git-backend", "requested auto", "", OK},
		{"state-lock", "state lock available", "", OK},
	})
	if r.Code(false) != apply.Warnings {
		t.Fatal("manifest failure exit", r.Code(false))
	}
}

type manifestCase struct {
	env                           map[string]string
	name, manifest, version, flag string
	dirs, files, order            []string
	expects                       []expect
	code                          apply.Code
}

func manifestVersionCases() []manifestCase {
	return []manifestCase{
		{
			name: "invalid host flag", manifest: "schema=1\n", flag: "not a host!",
			expects: []expect{{"manifest", "invalid configuration host", "host selection", Fail}}, code: apply.Warnings,
		},
		{
			name: "tool too old", manifest: "schema=1\nmin_tool_version=\"2.0.0\"\n", version: "1.0.0",
			expects: []expect{{"tool-version", "kitbag 1.0.0; min_tool_version 2.0.0", "upgrade kitbag", Fail}}, code: apply.ToolTooOld,
		},
		{
			name: "tool new enough", manifest: "schema=1\nmin_tool_version=\"0.5.0\"\n", version: "1.0.0",
			expects: []expect{{"tool-version", "kitbag 1.0.0; min_tool_version 0.5.0", "", OK}},
		},
	}
}

func manifestInstanceCases() []manifestCase {
	return []manifestCase{
		{
			name: "default instance missing", manifest: codexManifest,
			expects: []expect{{"instance-codex/default", codexHome(), "create or correct instance directory " + codexHome(), Warn}},
		},
		{
			name: "default instance present", manifest: codexManifest, dirs: []string{codexHome()},
			expects: []expect{{"instance-codex/default", codexHome(), "", OK}},
		},
		{
			name: "instance is a file", manifest: codexManifest, files: []string{codexHome()},
			expects: []expect{{"instance-codex/default", "not a directory", "create or correct", Warn}},
		},
		{
			name: "relocated default", manifest: "schema=1\n[targets.claude]\nagent=\"claude\"\n",
			env:     map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(testHome(), "relocated")},
			expects: []expect{{"instance-claude/default", filepath.Join(testHome(), "relocated"), "create or correct", Warn}},
		},
		{
			name: "relative override", manifest: codexManifest, env: map[string]string{"CODEX_HOME": "relative"},
			expects: []expect{{"instances", "cannot resolve instance directories", "use absolute CODEX_HOME/CLAUDE_CONFIG_DIR", Fail}}, code: apply.Warnings,
		},
		{
			name: "listed instances sorted and filtered",
			manifest: codexManifest +
				"[targets.codex.instances.second]\ndir=\"~/second\"\n" +
				"[targets.codex.instances.first]\ndir=\"~/first\"\n" +
				"[targets.codex.instances.off]\ndir=\"~/off\"\nenabled=false\n" +
				"[targets.disabled]\nagent=\"claude\"\nenabled=false\n" +
				"[targets.plain]\nagent=\"generic\"\n" +
				"[targets.plain.instances.main]\ndir=\"~/plain\"\nroots={skills=\"~/plain/skills\"}\n",
			dirs:  []string{filepath.Join(testHome(), "first")},
			order: []string{"tool-version", "instance-codex/first", "instance-codex/second", "instance-plain/main"},
			expects: []expect{
				{"instance-codex/first", filepath.Join(testHome(), "first"), "", OK},
				{"instance-codex/second", filepath.Join(testHome(), "second"), "create or correct", Warn},
			},
		},
		{
			name:     "host flag selects override",
			manifest: codexManifest + "[hosts.laptop.targets.codex]\nenabled=false\n",
			flag:     "laptop", order: []string{"tool-version"},
		},
	}
}

func TestManifestChecks(t *testing.T) {
	t.Parallel()
	for _, tc := range append(manifestVersionCases(), manifestInstanceCases()...) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, host := subjectWith(t, tc.env, tc.manifest)
			if tc.version != "" {
				s.Version = tc.version
			}
			s.HostFlag = tc.flag
			for _, dir := range tc.dirs {
				must(t, host.MkdirAll(dir, probeDirPerm))
			}
			for _, file := range tc.files {
				must(t, host.WriteAtomic(file, []byte("x"), probeFilePerm))
			}
			r := Report{Checks: s.manifestChecks(t.Context())}
			assertChecks(t, r, tc.order, tc.expects)
			if got := r.Code(false); got != tc.code {
				t.Fatalf("code = %d, want %d", got, tc.code)
			}
		})
	}
}

type gitCase struct {
	backend       vcs.Backend
	openErr       error
	resolution    vcs.Resolution
	name          string
	order, absent []string
	expects       []expect
}

func gitCases() []gitCase {
	systemResolution := vcs.Resolution{Selection: vcs.Selection{Kind: vcs.Auto, Source: vcs.SourceDefault}, Active: vcs.System}
	return []gitCase{
		{
			name: "open fails", openErr: errors.New("no repository"), order: []string{"git-backend"},
			expects: []expect{{"git-backend", "no repository", vcs.SystemHint, Fail}},
		},
		{
			name:       "automatic fallback warns",
			backend:    &diagnosticBackend{cfg: vcs.CommitConfig{Identity: true}},
			resolution: vcs.Resolution{Selection: systemResolution.Selection, Active: vcs.System, SystemErr: errors.New("git too old")},
			expects:    []expect{{"git-backend", "system Git unavailable: git too old", vcs.SystemHint + " to use system Git", Warn}},
		},
		{
			name: "remote error is not echoed", resolution: systemResolution,
			backend: &diagnosticBackend{cfg: vcs.CommitConfig{Identity: true}, remoteErr: errors.New("https://user:hunter2@example.invalid/repo")},
			expects: []expect{{"remote", "origin is unreachable", "check origin URL", Fail}},
			absent:  []string{"hunter2", "example.invalid"},
		},
		{
			name: "embedded backend skips probes", backend: embeddedBackend{},
			resolution: vcs.Resolution{Selection: systemResolution.Selection, Active: vcs.Embedded},
			order:      []string{"git-backend", "remote", "commit-config"},
			expects: []expect{
				{"remote", "backend cannot probe remote", "--git-backend=system", Warn},
				{"commit-config", "skipped", "", OK},
			},
		},
		{
			name: "commit config unreadable", resolution: systemResolution,
			backend: &diagnosticBackend{inspectErr: errors.New("bad config value secret=hunter2")},
			order:   []string{"git-backend", "remote", "commit-config"},
			expects: []expect{{"commit-config", "cannot read Git commit configuration", "repair the working copy", Warn}},
			absent:  []string{"hunter2"},
		},
		{
			name: "identity and signing missing", resolution: systemResolution,
			backend: &diagnosticBackend{cfg: vcs.CommitConfig{Signing: true}},
			expects: []expect{
				{"commit-identity", "user.name or user.email is missing", "set git config user.name", Warn},
				{"commit-signing", "commit.gpgsign is enabled without user.signingkey", "set git config user.signingkey", Warn},
			},
		},
		{
			name: "signing with key", resolution: systemResolution,
			backend: &diagnosticBackend{cfg: vcs.CommitConfig{Identity: true, Signing: true, SigningKey: true}},
			order:   []string{"git-backend", "remote", "commit-identity", "commit-signing"},
			expects: []expect{
				{"git-backend", "active system", "", OK},
				{"remote", "remote reachability", "", OK},
				{"commit-identity", "commit identity configured", "", OK},
				{"commit-signing", "signing configuration checked", "", OK},
			},
		},
	}
}

func TestGitChecks(t *testing.T) {
	t.Parallel()
	for _, tc := range gitCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := subject(t)
			s.Open = func(ctx context.Context) (vcs.Backend, vcs.Resolution, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("open without timeout")
				}
				return tc.backend, tc.resolution, tc.openErr
			}
			assertChecks(t, Report{Checks: s.gitChecks(t.Context())}, tc.order, tc.expects, tc.absent...)
		})
	}
}

// embeddedBackend can neither probe remotes nor inspect commit configuration.
type embeddedBackend struct{ vcs.Backend }

// Kind identifies the embedded backend.
func (embeddedBackend) Kind() vcs.Kind { return vcs.Embedded }

func TestSSHCheck(t *testing.T) {
	t.Parallel()
	sock := filepath.FromSlash("/run/agent.sock")
	set := map[string]string{"SSH_AUTH_SOCK": sock}
	const fix = "start an SSH agent and export SSH_AUTH_SOCK"
	for _, tc := range []struct {
		env          map[string]string
		name         string
		want         expect
		file, socket bool
	}{
		{name: "unset", want: expect{"ssh-agent", "SSH_AUTH_SOCK does not name a socket", fix, Warn}},
		{name: "missing", env: set, want: expect{"ssh-agent", "SSH agent socket", fix, Warn}},
		{name: "regular file", env: set, file: true, want: expect{"ssh-agent", "SSH_AUTH_SOCK does not name a socket", fix, Warn}},
		{name: "socket", env: set, socket: true, want: expect{"ssh-agent", "SSH agent socket", "", OK}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, host := subjectWith(t, tc.env, "schema=1\n")
			if tc.file {
				must(t, host.MkdirAll(filepath.Dir(sock), probeDirPerm))
				must(t, host.WriteAtomic(sock, nil, probeFilePerm))
			}
			if tc.socket {
				s.Host = socketHost{Host: host, path: sock}
			}
			assertChecks(t, Report{Checks: s.sshChecks(t.Context())}, []string{"ssh-agent"}, []expect{tc.want})
		})
	}
}

// socketHost reports path as a Unix socket, which the fake cannot create.
type socketHost struct {
	*fake.Host
	path string
}

// Stat returns socket metadata for the configured path.
func (h socketHost) Stat(path string) (fs.FileInfo, error) {
	if path == h.path {
		return socketInfo{}, nil
	}
	return h.Host.Stat(path)
}

type socketInfo struct{ fs.FileInfo }

// Mode marks the entry as a socket.
func (socketInfo) Mode() fs.FileMode { return fs.ModeSocket }

func TestAgentChecks(t *testing.T) {
	t.Parallel()
	s, _ := subject(t)
	var probed []string
	s.VersionProbe = func(ctx context.Context, name string) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > Timeout {
			t.Errorf("%s probe deadline %v, want at most %v", name, deadline, Timeout)
		}
		probed = append(probed, name)
		if name == "claude" {
			return "", errors.New("exit status 7: token=hunter2")
		}
		return "codex-cli\n  1.2.3 ", nil
	}
	r := s.Run(t.Context())
	if !slices.Equal(probed, []string{"claude", "codex"}) {
		t.Fatalf("probed %q", probed)
	}
	// Run collapses whitespace so every message stays on one line.
	assertChecks(t, r, nil, []expect{
		{"agent-claude", "binary absent or --version probe failed/timed out", "install claude or check its PATH", Warn},
		{"agent-codex", "codex codex-cli 1.2.3", "", OK},
	}, "hunter2")
	if r.Code(false) != apply.OK || r.Code(true) != apply.Warnings {
		t.Fatalf("agent warning exit = %d/%d", r.Code(false), r.Code(true))
	}
}

// legacySubject enables legacy inspection with a codex target and one old root.
func legacySubject(t *testing.T, env map[string]string) (Service, *fake.Host) {
	t.Helper()
	s, host := subjectWith(t, env, codexManifest)
	s.Config.Migration.LegacyRoots = []string{"~/old"}
	return s, host
}

func TestLegacyDisabledWithoutRoots(t *testing.T) {
	t.Parallel()
	s, _ := subject(t)
	assertChecks(t, Report{Checks: s.legacyChecks(t.Context())}, []string{"legacy"}, []expect{
		{"legacy", "skipped: migration.legacy_roots is empty", "", OK},
	})
}

func TestLegacyHomeUnavailable(t *testing.T) {
	t.Parallel()
	s, host := legacySubject(t, nil)
	s.Host = homelessHost{host}
	assertChecks(t, Report{Checks: s.legacyChecks(t.Context())}, []string{"legacy"}, []expect{
		{"legacy", "legacy inspection unavailable", "repair HOME", Warn},
	})
}

// homelessHost cannot resolve HOME.
type homelessHost struct{ *fake.Host }

// HomeDir always fails.
func (homelessHost) HomeDir() (string, error) { return "", errors.New("HOME is not set") }

func TestLegacyClean(t *testing.T) {
	t.Parallel()
	s, host := legacySubject(t, nil)
	must(t, host.MkdirAll(filepath.Join(codexHome(), "skills", "mine"), probeDirPerm))
	must(t, host.WriteAtomic(filepath.Join(codexHome(), "AGENTS.md"), []byte("# own instructions\n"), probeFilePerm))
	must(t, host.MkdirAll(filepath.Join(testHome(), ".config", "systemd", "user", "other.timer"), probeDirPerm))
	assertChecks(t, Report{Checks: s.legacyChecks(t.Context())}, []string{"legacy-links", "legacy-markers", "legacy-settings-markers", "legacy-units"}, []expect{
		{"legacy-links", "legacy links", "", OK},
		{"legacy-markers", "legacy instruction markers", "", OK},
		{"legacy-settings-markers", "legacy hook markers", "", OK},
		{"legacy-units", "legacy timer/service definitions", "", OK},
	})
}

func TestLegacyLeftovers(t *testing.T) {
	t.Parallel()
	xdg := filepath.Join(testHome(), "xdg")
	s, host := legacySubject(t, map[string]string{"XDG_CONFIG_HOME": xdg})
	old, agents := filepath.Join(testHome(), "old"), filepath.Join(testHome(), ".agents", "skills")
	must(t, host.MkdirAll(old, probeDirPerm))
	must(t, host.MkdirAll(filepath.Join(codexHome(), "skills"), probeDirPerm))
	must(t, host.MkdirAll(agents, probeDirPerm))
	linked, shared := filepath.Join(codexHome(), "skills", "review"), filepath.Join(agents, "shared")
	must(t, host.Link(filepath.Join(old, "skills", "review"), linked))
	// The default .agents/skills root marks links into itself as legacy.
	must(t, host.Link(filepath.Join(agents, "elsewhere"), shared))
	agentsMD, claudeMD := filepath.Join(codexHome(), "AGENTS.md"), filepath.Join(codexHome(), "CLAUDE.md")
	must(t, host.WriteAtomic(agentsMD, []byte("<!-- agent-config-managed -->\n"), probeFilePerm))
	must(t, host.WriteAtomic(claudeMD, []byte("Managed by agent-config-sync\n"), probeFilePerm))
	unit := filepath.Join(xdg, "systemd", "user", "agent-config-sync.timer")
	launch := filepath.Join(testHome(), "Library", "LaunchAgents", "com.example.agent-config.plist")
	for _, path := range []string{unit, launch} {
		must(t, host.MkdirAll(filepath.Dir(path), probeDirPerm))
		must(t, host.WriteAtomic(path, nil, probeFilePerm))
	}
	r := Report{Checks: s.legacyChecks(t.Context())}
	const linkFix, markerFix, unitFix = "migrate/adopt them explicitly", "review legacy managed instructions", "disable old agent-config-sync timers/services"
	assertChecks(t, r, nil, []expect{
		{"legacy-links", linked, linkFix, Warn},
		{"legacy-links", shared, linkFix, Warn},
		{"legacy-markers", agentsMD, markerFix, Warn},
		{"legacy-markers", claudeMD, markerFix, Warn},
		{"legacy-units", unit, unitFix, Warn},
		{"legacy-units", launch, unitFix, Warn},
	})
	// Redaction maps the throwaway HOME to ~ in every serialized field.
	assertChecks(t, r.Redact(testHome()), nil, []expect{
		{"legacy-units", filepath.Join("~", "Library", "LaunchAgents"), unitFix, Warn},
	}, testHome())
}

func TestLegacyUnreadableInstructions(t *testing.T) {
	t.Parallel()
	s, host := legacySubject(t, nil)
	// A directory named AGENTS.md cannot be read as instructions.
	must(t, host.MkdirAll(filepath.Join(codexHome(), "AGENTS.md"), probeDirPerm))
	assertChecks(t, Report{Checks: s.legacyChecks(t.Context())}, nil, []expect{
		{"legacy-markers", "AGENTS.md", "review legacy managed instructions", Warn},
		{"legacy-links", "legacy links", "", OK},
	})
}

func TestMaterialChecks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		host    func(*fake.Host) platform.Host
		name    string
		mode    string
		order   []string
		expects []expect
	}{
		{name: "healthy", expects: []expect{
			{"materialization-mode", "materialization mode link", "", OK},
			{"copy", "atomic copy", "", OK},
			{"link", "symbolic links", "", OK},
		}},
		{name: "invalid mode", mode: "hardlink", expects: []expect{
			{"materialization-mode", "materialization mode", "select --mode=link or --mode=copy", Fail},
		}},
		{
			name: "no dirs", host: func(h *fake.Host) platform.Host { return noDirsHost{h} }, order: []string{"materialization"},
			expects: []expect{{"materialization", "cannot resolve probe directory", "repair XDG directory configuration", Fail}},
		},
		{name: "cache unwritable", host: func(h *fake.Host) platform.Host { return noWriteHost{h} }, expects: []expect{
			{"copy", "atomic copy", "make the cache directory writable", Fail}, {"link", "symbolic links", "", OK},
		}},
		{name: "link target mismatch", host: func(h *fake.Host) platform.Host { return wrongLinkHost{h} }, expects: []expect{
			{"link", fs.ErrInvalid.Error(), "use --mode=copy", Fail},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, host := subject(t)
			if tc.host != nil {
				s.Host = tc.host(host)
			}
			s.ModeFlag = tc.mode
			assertChecks(t, Report{Checks: s.materialChecks()}, tc.order, tc.expects)
		})
	}
}

type noDirsHost struct{ *fake.Host }

// Dirs fails as with an invalid XDG configuration.
func (noDirsHost) Dirs() (platform.Dirs, error) {
	return platform.Dirs{}, errors.New("relative XDG_CACHE_HOME")
}

type noWriteHost struct{ *fake.Host }

// WriteAtomic simulates a read-only cache filesystem.
func (noWriteHost) WriteAtomic(string, []byte, fs.FileMode) error { return fs.ErrPermission }

type wrongLinkHost struct{ *fake.Host }

// ReadLink reports a target other than the one just created.
func (wrongLinkHost) ReadLink(string) (string, error) { return filepath.FromSlash("/elsewhere"), nil }

func TestExitCodeCombinations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		checks         []Check
		normal, strict apply.Code
	}{
		{"empty", nil, apply.OK, apply.OK},
		{"tool-version warning is ordinary", []Check{{ID: "tool-version", Status: Warn}}, apply.OK, apply.Warnings},
		{"old tool wins after failure", []Check{{ID: "remote", Status: Fail}, {ID: "tool-version", Status: Fail}}, apply.ToolTooOld, apply.ToolTooOld},
		{"old tool wins before failure", []Check{{ID: "tool-version", Status: Fail}, {ID: "remote", Status: Fail}, {ID: "ssh-agent", Status: Warn}}, apply.ToolTooOld, apply.ToolTooOld},
		{"warnings and ok", []Check{{ID: "remote", Status: OK}, {ID: "ssh-agent", Status: Warn}}, apply.OK, apply.Warnings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := Report{Checks: tc.checks}
			if got := r.Code(false); got != tc.normal {
				t.Errorf("normal = %d, want %d", got, tc.normal)
			}
			if got := r.Code(true); got != tc.strict {
				t.Errorf("strict = %d, want %d", got, tc.strict)
			}
		})
	}
}
