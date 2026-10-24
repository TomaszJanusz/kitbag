package platform_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s suite) runPermissions(t *testing.T) {
	t.Helper()
	t.Run("Permissions/WriteDenied", s.permissionWriteDenied)
	t.Run("Permissions/ReadDenied", s.permissionReadDenied)
}

// restoreMode lets t.TempDir remove a directory the test made read-only.
func restoreMode(t *testing.T, h platform.FS, path string) {
	t.Helper()
	t.Cleanup(func() {
		if err := h.Chmod(path, 0o700); err != nil {
			t.Errorf("restore mode of %s: %v", path, err)
		}
	})
}

func (s suite) permissionWriteDenied(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	if !sub.permissions {
		t.Skip("permission bits are not enforced for this user")
	}
	h, dir := sub.host, filepath.Join(sub.root, "read-only")
	mustMkdir(t, h, dir)
	mustWrite(t, h, filepath.Join(dir, "f"), "f", 0o644)
	mustWrite(t, h, filepath.Join(sub.root, "g"), "g", 0o644)
	mustNil(t, "chmod", h.Chmod(dir, 0o500))
	restoreMode(t, h, dir)

	file := filepath.Join(dir, "f")
	for op, err := range map[string]error{
		"WriteAtomic new":      h.WriteAtomic(filepath.Join(dir, "new"), nil, 0o644),
		"WriteAtomic existing": h.WriteAtomic(file, []byte("changed"), 0o644),
		"Link":                 h.Link(file, filepath.Join(dir, "l")),
		"MkdirAll":             h.MkdirAll(filepath.Join(dir, "sub"), 0o755),
		"Remove":               h.Remove(file),
		"Rename out":           h.Rename(file, filepath.Join(sub.root, "out")),
		"Rename in":            h.Rename(filepath.Join(sub.root, "g"), filepath.Join(dir, "g")),
	} {
		wantErr(t, op, err, fs.ErrPermission)
	}
	_, err := h.TryLock(filepath.Join(dir, "lock"))
	wantErr(t, "TryLock new", err, fs.ErrPermission)
	wantContent(t, h, file, "f")
}

func (s suite) permissionReadDenied(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	if !sub.permissions {
		t.Skip("permission bits are not enforced for this user")
	}
	h := sub.host
	secret := filepath.Join(sub.root, "secret")
	mustWrite(t, h, secret, "x", 0o200)
	unlisted, closed := filepath.Join(sub.root, "unlisted"), filepath.Join(sub.root, "closed")
	mustMkdir(t, h, unlisted)
	mustMkdir(t, h, closed)
	mustWrite(t, h, filepath.Join(closed, "f"), "f", 0o644)
	mustNil(t, "chmod unlisted", h.Chmod(unlisted, 0o300))
	mustNil(t, "chmod closed", h.Chmod(closed, 0o600))
	restoreMode(t, h, unlisted)
	restoreMode(t, h, closed)

	_, err := h.ReadFile(secret)
	wantErr(t, "ReadFile 0200", err, fs.ErrPermission)
	_, err = h.ReadDir(unlisted)
	wantErr(t, "ReadDir 0300", err, fs.ErrPermission)
	_, err = h.Lstat(filepath.Join(closed, "f"))
	wantErr(t, "Lstat below 0600", err, fs.ErrPermission)
	_, err = h.TryLock(secret)
	wantErr(t, "TryLock 0200", err, fs.ErrPermission)
}

func (s suite) runLocks(t *testing.T) {
	t.Helper()
	t.Run("TryLock/Exclusive", s.tryLockExclusive)
	t.Run("TryLock/KeepsExistingFile", s.tryLockKeepsExistingFile)
	t.Run("TryLock/Errors", s.tryLockErrors)
}

func (s suite) tryLockExclusive(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "state.lock")
	first, err := h.TryLock(path)
	mustNil(t, "first lock", err)
	_, err = h.TryLock(path)
	wantErr(t, "second lock", err, platform.ErrLocked)
	mustNil(t, "unlock", first.Unlock())
	again, err := h.TryLock(path)
	mustNil(t, "lock after unlock", err)
	mustNil(t, "unlock again", again.Unlock())
	wantErr(t, "double unlock", again.Unlock(), fs.ErrClosed)
	wantKind(t, h, path, 0, 0o600)
}

func (s suite) tryLockKeepsExistingFile(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "lock")
	mustWrite(t, h, path, "pid 42", 0o644)
	l, err := h.TryLock(path)
	mustNil(t, "lock", err)
	mustNil(t, "unlock", l.Unlock())
	wantContent(t, h, path, "pid 42")
	wantKind(t, h, path, 0, 0o644)
}

func (s suite) tryLockErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	link := filepath.Join(sub.root, "link")
	mustLink(t, h, file, link)
	for _, tc := range []struct {
		want error
		path string
	}{
		{path: filepath.Join(sub.root, "missing", "lock"), want: fs.ErrNotExist},
		{path: sub.root, want: platform.ErrIsDir},
		{path: link, want: fs.ErrInvalid},
	} {
		_, err := h.TryLock(tc.path)
		wantErr(t, "TryLock "+tc.path, err, tc.want)
	}
	wantNames(t, h, sub.root, "file", "link")
}

func (s suite) runEnv(t *testing.T) {
	t.Helper()
	t.Run("Env/LookupEnv", s.envLookup)
	t.Run("Env/DirsDefaultToHome", s.envDirsDefault)
	t.Run("Env/DirsHonorAbsoluteXDG", s.envDirsXDG)
	t.Run("Env/HomeRequired", s.envHomeRequired)
	t.Run("Env/AgentHome", s.envAgentHome)
	t.Run("Env/AgentHomeErrors", s.envAgentHomeErrors)
	t.Run("Env/Hostname", s.envHostname)
	t.Run("Env/OpenValidatesTargets", s.envOpen)
	t.Run("Env/IsTerminal", s.envIsTerminal)
}

// home is an absolute home directory used only as a string.
func home(parts ...string) string {
	return filepath.Join(append([]string{filepath.FromSlash("/home/kit")}, parts...)...)
}

func (s suite) envLookup(t *testing.T) {
	t.Parallel()
	h := s.newSubject(t, map[string]string{"SET": "value", "EMPTY": ""}).host
	for key, want := range map[string]struct {
		value string
		ok    bool
	}{"SET": {"value", true}, "EMPTY": {"", true}, "UNSET": {"", false}} {
		if value, ok := h.LookupEnv(key); value != want.value || ok != want.ok {
			t.Errorf("LookupEnv(%s) = %q, %v; want %q, %v", key, value, ok, want.value, want.ok)
		}
	}
}

func wantDirs(t *testing.T, h platform.Env, want platform.Dirs) {
	t.Helper()
	if got, err := h.Dirs(); err != nil || got != want {
		t.Errorf("Dirs() = %+v, %v; want %+v", got, err, want)
	}
}

func (s suite) envDirsDefault(t *testing.T) {
	t.Parallel()
	want := platform.Dirs{
		Config: home(".config", "kitbag"),
		Data:   home(".local", "share", "kitbag"),
		State:  home(".local", "state", "kitbag"),
		Cache:  home(".cache", "kitbag"),
	}
	wantDirs(t, s.newSubject(t, map[string]string{"HOME": home()}).host, want)
	// The XDG specification ignores empty and relative values.
	wantDirs(t, s.newSubject(t, map[string]string{
		"HOME":            home() + string(filepath.Separator),
		"XDG_CONFIG_HOME": "",
		"XDG_DATA_HOME":   "relative",
		"XDG_STATE_HOME":  filepath.Join(".", "state"),
	}).host, want)
}

func (s suite) envDirsXDG(t *testing.T) {
	t.Parallel()
	xdg := filepath.FromSlash("/xdg")
	h := s.newSubject(t, map[string]string{
		"HOME":            home(),
		"XDG_CONFIG_HOME": filepath.Join(xdg, "config"),
		"XDG_DATA_HOME":   filepath.Join(xdg, "data") + string(filepath.Separator),
		"XDG_STATE_HOME":  filepath.Join(xdg, "state"),
		"XDG_CACHE_HOME":  filepath.Join(xdg, "cache"),
	}).host
	wantDirs(t, h, platform.Dirs{
		Config: filepath.Join(xdg, "config", "kitbag"),
		Data:   filepath.Join(xdg, "data", "kitbag"),
		State:  filepath.Join(xdg, "state", "kitbag"),
		Cache:  filepath.Join(xdg, "cache", "kitbag"),
	})
	if got, err := h.HomeDir(); err != nil || got != home() {
		t.Errorf("HomeDir() = %q, %v; want %q", got, err, home())
	}
}

func (s suite) envHomeRequired(t *testing.T) {
	t.Parallel()
	for _, env := range []map[string]string{nil, {"HOME": ""}, {"HOME": "relative"}} {
		h := s.newSubject(t, env).host
		if _, err := h.HomeDir(); err == nil {
			t.Errorf("HomeDir() with %v succeeded, want an error", env)
		}
		if _, err := h.Dirs(); err == nil {
			t.Errorf("Dirs() with %v succeeded, want an error", env)
		}
		if _, err := h.AgentHome("CODEX_HOME", ".codex"); err == nil {
			t.Errorf("AgentHome() with %v succeeded, want an error", env)
		}
	}
	_, err := s.newSubject(t, map[string]string{"HOME": "relative"}).host.HomeDir()
	wantErr(t, "relative HOME", err, fs.ErrInvalid)
}

func (s suite) envAgentHome(t *testing.T) {
	t.Parallel()
	codex := filepath.FromSlash("/opt/codex")
	for _, tc := range []struct {
		name          string
		env           map[string]string
		override, rel string
		want          string
	}{
		{env: map[string]string{"HOME": home()}, override: "CODEX_HOME", rel: ".codex", want: home(".codex"), name: "unset"},
		{env: map[string]string{"HOME": home(), "CODEX_HOME": ""}, override: "CODEX_HOME", rel: ".codex", want: home(".codex"), name: "empty"},
		{env: map[string]string{"HOME": home(), "CODEX_HOME": codex + string(filepath.Separator)}, override: "CODEX_HOME", rel: ".codex", want: codex, name: "override"},
		{env: map[string]string{"CODEX_HOME": codex}, override: "CODEX_HOME", rel: ".codex", want: codex, name: "override without HOME"},
		{env: map[string]string{"HOME": home()}, rel: ".agents/skills", want: home(".agents", "skills"), name: "no override"},
	} {
		got, err := s.newSubject(t, tc.env).host.AgentHome(tc.override, tc.rel)
		if err != nil || got != tc.want {
			t.Errorf("AgentHome %s = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func (s suite) envAgentHomeErrors(t *testing.T) {
	t.Parallel()
	h := s.newSubject(t, map[string]string{"HOME": home(), "CODEX_HOME": "relative/codex"}).host
	for _, tc := range [][2]string{{"CODEX_HOME", ".codex"}, {"", "../escape"}, {"", home("abs")}, {"", ""}} {
		_, err := h.AgentHome(tc[0], tc[1])
		wantErr(t, "AgentHome("+tc[0]+", "+tc[1]+")", err, fs.ErrInvalid)
	}
}

func (s suite) envHostname(t *testing.T) {
	t.Parallel()
	if name, err := s.subject(t).host.Hostname(); err != nil || name == "" {
		t.Errorf("Hostname() = %q, %v; want a name", name, err)
	}
}

func (s suite) envOpen(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	valid := []string{sub.root, "https://example.com/docs?q=1", "http://127.0.0.1:8080/"}
	for _, target := range valid {
		mustNil(t, "Open "+target, sub.host.Open(target))
	}
	for _, target := range []string{
		"", "relative", "-a", "--help", raw(sub.root, "..", "x"), "ftp://example.com",
		"file:///etc/passwd", "https:///no-host", "javascript:alert(1)", "vscode://open",
	} {
		wantErr(t, "Open "+target, sub.host.Open(target), fs.ErrInvalid)
	}
	if got := sub.opened(); !slices.Equal(got, valid) {
		t.Errorf("opened = %q, want %q", got, valid)
	}
}

func (s suite) envIsTerminal(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	if sub.host.IsTerminal(sub.nonTerminal) {
		t.Errorf("IsTerminal(%d) = true for a non-terminal", sub.nonTerminal)
	}
}
