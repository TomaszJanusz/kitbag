package agents_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
)

// base2 and two2 are schema-2 versions of the fixtures; mac is this machine
// (prepare's HostName).
func base2() string {
	return strings.Replace(base, "schema = 1", "schema = 2", 1) + "\n[hosts.runner02]\nmode = \"copy\"\n"
}

func two2() string { return strings.Replace(two, "schema = 1", "schema = 2", 1) }

func TestAddWithHosts(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.Hosts = []string{"mac", "runner02"}
	p, err := prepare(t, nil, base2(), change)
	expect(t, err == nil, "error", err)
	expect(t, strings.Contains(string(p.After), "[targets.claude.instances.second]\ndir = \"~/.claude_2\"\nhosts = [\"mac\", \"runner02\"]\n"), "after", string(p.After))
	// The full subject is too long for a title; the body keeps it.
	expect(t, p.Title == "chore(kitbag): add claude instance second" && strings.Contains(p.Message, "- add claude instance second at ~/.claude_2 (hosts mac, runner02)\n"), "message", p.Message)
	expect(t, slices.Equal(p.Change.Hosts, []string{"mac", "runner02"}) && len(p.Warnings) == 0, "plan", p)
}

func TestAddForOtherHostsSkipsThisMachine(t *testing.T) {
	t.Parallel()
	change := add("work", "~/.claude-work")
	change.Hosts = []string{"runner02"}
	p, err := prepare(t, nil, base2(), change)
	expect(t, err == nil, "error", err)
	// ~/.claude-work is missing here, but the instance does not apply here.
	expect(t, slices.Equal(p.Warnings, []string{"~/.claude-work is not used on this machine (mac), which its hosts list leaves out"}), "warnings", p.Warnings)
}

func TestHostsWarnAboutUnknownNames(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.Hosts = []string{"mac", "runer02"}
	p, err := prepare(t, nil, base2(), change)
	expect(t, err == nil, "error", err)
	expect(t, slices.Contains(p.Warnings, "host runer02 is neither this machine (mac) nor declared under [hosts]; check its spelling"), "warnings", p.Warnings)
}

func TestHostsRefusals(t *testing.T) {
	t.Parallel()
	withHosts := func(c agents.Change, all bool, hosts ...string) agents.Change {
		c.Hosts, c.AllHosts = hosts, all
		return c
	}
	edit := agents.Change{Op: agents.Edit, Target: "claude", Name: "second"}
	patched := two2() + "\n[hosts.runner02.targets.claude.instances.second]\nsettings = false\n"
	for _, tc := range []struct {
		kind           error
		name, manifest string
		want           string
		change         agents.Change
	}{
		{agents.ErrUnsupported, "schema 1 add", base, "needs manifest schema 2, and this kitbag.toml uses schema 1; preview the upgrade with kitbag manifest upgrade --dry-run", withHosts(add("second", "~/.claude_2"), false, "mac")},
		{agents.ErrUnsupported, "schema 1 edit", two, "needs manifest schema 2", withHosts(edit, false, "mac")},
		{agents.ErrInvalid, "invalid host name", base2(), `host "my mac"`, withHosts(add("second", "~/.claude_2"), false, "my mac")},
		{agents.ErrInvalid, "duplicate host", base2(), "host mac is listed twice", withHosts(add("second", "~/.claude_2"), false, "mac", "mac")},
		{agents.ErrInvalid, "both scopes", base2(), "either a hosts list or all hosts", withHosts(add("second", "~/.claude_2"), true, "mac")},
		{agents.ErrInvalid, "patch left outside", patched, "[hosts.runner02.targets.claude.instances.second] patches this instance on runner02", withHosts(edit, false, "mac")},
		{agents.ErrInvalid, "remove patched", patched, "delete that table by hand first", agents.Change{Op: agents.Remove, Target: "claude", Name: "second"}},
		{agents.ErrInvalid, "unchanged", two2(), "nothing to change", withHosts(edit, true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := prepare(t, nil, tc.manifest, tc.change)
			expect(t, errors.Is(err, tc.kind) && errors.Is(err, fs.ErrInvalid) && strings.Contains(err.Error(), tc.want), "error", err)
		})
	}
}

func TestEditHosts(t *testing.T) {
	t.Parallel()
	restrict := agents.Change{Op: agents.Edit, Target: "claude", Name: "second", Hosts: []string{"mac"}}
	p, err := prepare(t, nil, two2(), restrict)
	expect(t, err == nil, "error", err)
	expect(t, p.Title == "chore(kitbag): edit claude instance second (hosts mac)", "title", p.Title)
	expect(t, slices.Contains(p.Diff, "+hosts = [\"mac\"]") && strings.Contains(string(p.After), "dir = \"~/.claude_2\"\nhosts = [\"mac\"]\n"), "diff", p.Diff)
	expect(t, p.Dropped == "", "dropped", p.Dropped)

	widen := agents.Change{Op: agents.Edit, Target: "claude", Name: "second", AllHosts: true}
	p, err = prepare(t, nil, string(p.After), widen)
	expect(t, err == nil, "error", err)
	expect(t, string(p.After) == two2() && p.Title == "chore(kitbag): edit claude instance second (all hosts)" && p.Change.AllHosts, "widened", p)
}

// TestSchema1AllHostsIsTheDefault accepts the explicit default scope on a
// schema-1 manifest without writing schema 2.
func TestSchema1AllHostsIsTheDefault(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.AllHosts = true
	p, err := prepare(t, nil, base, change)
	expect(t, err == nil && !strings.Contains(string(p.After), "hosts") && strings.HasPrefix(string(p.After), "schema = 1\n"), "plan", p)
}
