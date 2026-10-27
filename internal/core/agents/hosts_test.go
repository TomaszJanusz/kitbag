package agents_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
)

// declared adds a [hosts.runner02] table to base; mac is this machine
// (prepare's HostName).
const declared = base + "\n[hosts.runner02]\nmode = \"copy\"\n"

func TestAddWithHosts(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.Hosts = []string{"mac", "runner02"}
	p, err := prepare(t, nil, declared, change)
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
	p, err := prepare(t, nil, declared, change)
	expect(t, err == nil, "error", err)
	// ~/.claude-work is missing here, but the instance does not apply here.
	expect(t, slices.Equal(p.Warnings, []string{"~/.claude-work is not used on this machine (mac), which its hosts list leaves out"}), "warnings", p.Warnings)
}

func TestHostsWarnAboutUnknownNames(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.Hosts = []string{"mac", "runer02"}
	p, err := prepare(t, nil, declared, change)
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
	replaced := two + "\n[hosts.runner02.targets.claude.instances.main]\ndir = \"~/.claude\"\n"
	for _, tc := range []struct {
		kind           error
		name, manifest string
		want           string
		change         agents.Change
	}{
		{agents.ErrInvalid, "empty host name", declared, `host ""`, withHosts(add("second", "~/.claude_2"), false, "")},
		{agents.ErrInvalid, "invalid host name", declared, `host "my mac"`, withHosts(add("second", "~/.claude_2"), false, "my mac")},
		{agents.ErrInvalid, "duplicate host", declared, "host mac is listed twice", withHosts(add("second", "~/.claude_2"), false, "mac", "mac")},
		{agents.ErrInvalid, "both scopes", declared, "either a hosts list or all hosts", withHosts(add("second", "~/.claude_2"), true, "mac")},
		{agents.ErrUnsupported, "host replacement set", replaced, "[hosts.runner02.targets.claude.instances] replaces this target's instances on runner02, so agent edits would not reach it; move those instances to targets.claude.instances with a hosts list", withHosts(edit, false, "mac")},
		{agents.ErrUnsupported, "add to replaced target", replaced, "replaces this target's instances", withHosts(add("third", "~/.claude_3"), false, "mac")},
		{agents.ErrInvalid, "unchanged", two, "nothing to change", withHosts(edit, true)},
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
	p, err := prepare(t, nil, two, restrict)
	expect(t, err == nil, "error", err)
	expect(t, p.Title == "chore(kitbag): edit claude instance second (hosts mac)", "title", p.Title)
	expect(t, slices.Contains(p.Diff, "+hosts = [\"mac\"]") && strings.Contains(string(p.After), "dir = \"~/.claude_2\"\nhosts = [\"mac\"]\n"), "diff", p.Diff)
	expect(t, p.Dropped == "", "dropped", p.Dropped)

	widen := agents.Change{Op: agents.Edit, Target: "claude", Name: "second", AllHosts: true}
	p, err = prepare(t, nil, string(p.After), widen)
	expect(t, err == nil, "error", err)
	expect(t, string(p.After) == two && p.Title == "chore(kitbag): edit claude instance second (all hosts)" && p.Change.AllHosts, "widened", p)
}

// TestAllHostsIsTheDefault accepts the explicit default scope on add and
// writes no hosts key; the schema stays 1.
func TestAllHostsIsTheDefault(t *testing.T) {
	t.Parallel()
	change := add("second", "~/.claude_2")
	change.AllHosts = true
	p, err := prepare(t, nil, base, change)
	expect(t, err == nil && !strings.Contains(string(p.After), "hosts") && strings.HasPrefix(string(p.After), "schema = 1\n") && !p.Change.AllHosts, "plan", p)
}

// TestEditHostsKeepsLayout changes only the hosts line: comments, key order
// and the other instance stay byte for byte, together with a dir change.
func TestEditHostsKeepsLayout(t *testing.T) {
	t.Parallel()
	manifest := "# shared\nschema = 1\n\n[targets.claude]\nagent = \"claude\"\nsettings = \"claude/settings.json\"\n\n" +
		"[targets.claude.instances.main]\ndir = \"~/.claude\" # first\n\n# second account\n[targets.claude.instances.second]\n" +
		"settings = false\ndir = \"~/.claude_2\"\nhosts = [\"mac\"] # where it exists\nmode = \"copy\"\n"
	change := agents.Change{Op: agents.Edit, Target: "claude", Name: "second", Dir: "~/.claude_3", Hosts: []string{"mac", "runner02"}}
	p, err := prepare(t, nil, manifest, change)
	expect(t, err == nil, "error", err)
	want := strings.Replace(strings.Replace(manifest, `hosts = ["mac"]`, `hosts = ["mac", "runner02"]`, 1), `dir = "~/.claude_2"`, `dir = "~/.claude_3"`, 1)
	expect(t, string(p.After) == want, "after", string(p.After))
	expect(t, strings.Contains(p.Message, "- edit claude instance second (dir ~/.claude_3, hosts mac, runner02)\n") && p.Dropped == "~/.claude_2", "message", p.Message)
}
