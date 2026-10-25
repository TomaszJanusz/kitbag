package doctor

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

const claudeManifest = "schema=1\n[targets.claude]\nagent=\"claude\"\n"

// fakeSecret stands in for settings content that must never be reported.
const fakeSecret = "SETTINGS-CONTENT-CANARY-0123 hunter2"

func claudeSubject(t *testing.T) (Service, *fake.Host, string) {
	t.Helper()
	s, host := subjectWith(t, nil, claudeManifest)
	s.Config.Migration.LegacyRoots = []string{"~/old"}
	dir := filepath.Join(testHome(), ".claude")
	must(t, host.MkdirAll(dir, probeDirPerm))
	return s, host, filepath.Join(dir, "settings.json")
}

func settingsCheck(t *testing.T, s Service) Check {
	t.Helper()
	return find(t, Report{Checks: s.legacyChecks(t.Context())}, "legacy-settings-markers")
}

func TestLegacySettingsMarkerPresent(t *testing.T) {
	t.Parallel()
	s, host, path := claudeSubject(t)
	body := `{"token":"` + fakeSecret + `","hooks":{"a":"x # agent-config-managed","b":"y # agent-config-managed"}}`
	must(t, host.WriteAtomic(path, []byte(body), probeFilePerm))
	r := Report{Checks: s.legacyChecks(t.Context())}
	assertChecks(t, r, nil, []expect{
		{"legacy-settings-markers", path + ": 2 legacy hook marker occurrence(s)", "run kitbag adopt, then kitbag sync", Warn},
		{"legacy-links", "legacy links", "", OK},
	})
	if r.Code(false) != 0 || r.Code(true) != 1 {
		t.Fatalf("exit codes: normal %d, strict %d", r.Code(false), r.Code(true))
	}
	out, err := json.Marshal(r.Redact(testHome()))
	must(t, err)
	for _, leaked := range []string{fakeSecret, "CANARY", "hunter2", "token", testHome()} {
		if strings.Contains(string(out), leaked) {
			t.Errorf("report leaks %q: %s", leaked, out)
		}
	}
	// Unredacted text (stdout) names the path but never any content.
	if message := find(t, r, "legacy-settings-markers").Message; strings.Contains(message, "CANARY") || strings.Contains(message, "hunter2") {
		t.Errorf("message leaks content: %s", message)
	}
	if !strings.Contains(string(out), "~") || !strings.Contains(string(out), "settings.json") {
		t.Errorf("redacted path missing: %s", out)
	}
	after, err := host.ReadFile(path)
	must(t, err)
	if string(after) != body {
		t.Fatal("settings were modified")
	}
}

func TestLegacySettingsMarkerAbsentOrMissing(t *testing.T) {
	t.Parallel()
	s, host, path := claudeSubject(t)
	assertCheck(t, settingsCheck(t, s), expect{"legacy-settings-markers", "legacy hook markers", "", OK})
	must(t, host.WriteAtomic(path, []byte(`{"note":"`+fakeSecret+`","hooks":{"x":"# kitbag-managed"}}`), probeFilePerm))
	assertCheck(t, settingsCheck(t, s), expect{"legacy-settings-markers", "legacy hook markers", "", OK})
}

func TestLegacySettingsOnlyClaudeInstances(t *testing.T) {
	t.Parallel()
	s, host := legacySubject(t, nil)
	must(t, host.MkdirAll(codexHome(), probeDirPerm))
	must(t, host.WriteAtomic(filepath.Join(codexHome(), "settings.json"), []byte("# agent-config-managed"), probeFilePerm))
	assertCheck(t, settingsCheck(t, s), expect{"legacy-settings-markers", "legacy hook markers", "", OK})
}

func TestLegacySettingsOversized(t *testing.T) {
	t.Parallel()
	s, host, path := claudeSubject(t)
	big := append([]byte("# agent-config-managed"), make([]byte, settingsScanLimit)...)
	must(t, host.WriteAtomic(path, big, probeFilePerm))
	spy := &readSpy{Host: host}
	s.Host = spy
	assertCheck(t, settingsCheck(t, s), expect{"legacy-settings-markers", "too large to inspect", "smaller than 4 MiB", Warn})
	if spy.read {
		t.Fatal("oversized settings were read")
	}
}

// readSpy records reads of settings.json and can make them fail.
type readSpy struct {
	*fake.Host
	err  error
	read bool
}

// ReadFile records settings reads and optionally fails them.
func (r *readSpy) ReadFile(path string) ([]byte, error) {
	if filepath.Base(path) == "settings.json" {
		r.read = true
		if r.err != nil {
			return nil, r.err
		}
	}
	return r.Host.ReadFile(path)
}

func TestLegacySettingsUnreadable(t *testing.T) {
	t.Parallel()
	s, host, path := claudeSubject(t)
	must(t, host.WriteAtomic(path, []byte("# agent-config-managed "+fakeSecret), probeFilePerm))
	s.Host = &readSpy{Host: host, err: &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}}
	c := settingsCheck(t, s)
	assertCheck(t, c, expect{"legacy-settings-markers", "cannot inspect settings", "make settings.json readable", Warn})
	if strings.Contains(c.Message, "CANARY") {
		t.Fatal("content leaked", c.Message)
	}
}

func TestLegacySettingsDirectoryIsUnreadable(t *testing.T) {
	t.Parallel()
	s, host, path := claudeSubject(t)
	must(t, host.MkdirAll(path, probeDirPerm))
	assertCheck(t, settingsCheck(t, s), expect{"legacy-settings-markers", "cannot inspect settings", "make settings.json readable", Warn})
}
