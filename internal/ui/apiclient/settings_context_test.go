package apiclient

import (
	"context"
	"io/fs"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

type identityBackend struct {
	vcs.Backend
	err error
	id  vcs.Identity
}

func (b identityBackend) ReadIdentity(context.Context, string) (vcs.Identity, error) {
	return b.id, b.err
}

func TestSettingsContextUsesEffectiveWorkingCopy(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "KITBAG_REPO_PATH": "~/chosen"}))
	expect(t, host.MkdirAll("/home/test/chosen/hosts", 0o700) == nil, "mkdir")
	for _, name := range []string{"mac.md", "runner.md", "ignore.txt"} {
		expect(t, host.WriteAtomic("/home/test/chosen/hosts/"+name, nil, 0o600) == nil, "write %s", name)
	}
	expect(t, host.Link("/home/test/chosen/hosts/mac.md", "/home/test/chosen/hosts/link.md") == nil, "link")
	transport := &fakeTransport{results: map[string]any{"config.get": api.ConfigResult{Config: "schema=1\n[repo]\nurl='local'\npath='~/wrong'\n"}}}
	c := Client{Host: host, Env: host, Transport: transport, OpenGit: func(_ context.Context, kind string) (vcs.Backend, error) {
		expect(t, kind == "auto", "backend=%s", kind)
		return identityBackend{id: vcs.Identity{Name: "Git User", Email: "git@example.invalid"}}, nil
	}}
	values, err := c.Settings(t.Context())
	expect(t, err == nil, "settings=%v", err)
	byKey := indexSettings(values)
	expect(t, slices.Equal(byKey["host"].Hosts, []string{"mac", "runner"}), "hosts=%v", byKey["host"].Hosts)
	expect(t, byKey["git.identity.name"].GitValue == "Git User" && byKey["git.identity.email"].GitValue == "git@example.invalid", "Git hints=%v", byKey)
	expect(t, !byKey["migration.legacy_roots"].LegacyDetected, "legacy falsely detected")
	expect(t, host.MkdirAll("/home/test/src/agent-config", 0o700) == nil, "mkdir legacy")
	values, err = c.Settings(t.Context())
	expect(t, err == nil && indexSettings(values)["migration.legacy_roots"].LegacyDetected, "legacy not detected")
}

func indexSettings(values []model.SettingValue) map[string]model.SettingValue {
	result := make(map[string]model.SettingValue, len(values))
	for _, value := range values {
		result[value.Key] = value
	}
	return result
}

func TestSettingsContextFailureDoesNotPreventEditing(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	values := []model.SettingValue{{Key: "repo.url", Value: "local"}, {Key: "git.identity.name"}}
	for _, open := range []func(context.Context, string) (vcs.Backend, error){
		nil,
		func(context.Context, string) (vcs.Backend, error) { return nil, fs.ErrPermission },
		func(context.Context, string) (vcs.Backend, error) { return identityBackend{err: fs.ErrPermission}, nil },
	} {
		c := Client{Host: host, OpenGit: open}
		c.settingsContext(t.Context(), values)
		expect(t, values[0].Value == "local" && values[1].GitValue == "", "context corrupted settings=%v", values)
	}
}
