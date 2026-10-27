package manifestedit_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
)

// TestSetInstanceHosts rewrites, adds and deletes an instance's hosts list
// and changes nothing else in the file: comments and key order stay.
func TestSetInstanceHosts(t *testing.T) {
	t.Parallel()
	head := "# top\nschema = 1\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\" # dir\n"
	cases := []struct {
		name, input, want string
		hosts             []string
	}{
		{"added after dir", head + "settings = false\n", head + "hosts = [\"mac\"]\nsettings = false\n", []string{"mac"}},
		{"rewritten in place", head + "hosts = ['a'] # only a\nsettings = false\n", head + "hosts = [\"a\", \"b\"] # only a\nsettings = false\n", []string{"a", "b"}},
		{"deleted, comment kept", head + "hosts = [\"a\"] # only a\n\n[publish]\nmode = \"direct\"\n", head + "# only a\n\n[publish]\nmode = \"direct\"\n", nil},
		{"deleted at end of file without newline", head + "hosts = [\"a\"]", head, nil},
		{"absent and all hosts is a no-op", head, head, nil},
		{"crlf", "schema = 1\r\n[targets.c]\r\nagent = \"claude\"\r\n[targets.c.instances.x]\r\ndir = \"~/x\"\r\n", "schema = 1\r\n[targets.c]\r\nagent = \"claude\"\r\n[targets.c.instances.x]\r\ndir = \"~/x\"\r\nhosts = [\"mac\"]\r\n", []string{"mac"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifestedit.Apply("kitbag.toml", []byte(tc.input), manifestedit.Edit{Op: manifestedit.SetInstanceHosts, Target: "c", Name: "x", Hosts: tc.hosts})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// TestSetInstanceHostsRefusesOtherLayouts leaves lists it cannot rewrite
// safely to a hand edit.
func TestSetInstanceHostsRefusesOtherLayouts(t *testing.T) {
	t.Parallel()
	head := "schema = 1\n[targets.c]\nagent = \"claude\"\n[targets.c.instances.x]\ndir = \"~/x\"\n"
	for _, input := range []string{
		head + "hosts = [\n  \"a\",\n]\n",
		head + "hosts = [\"a\", # first\n \"b\"]\n",
		"schema = 1\n[targets.c]\nagent = \"claude\"\ninstances.x = { dir = \"~/x\" }\n",
	} {
		_, err := manifestedit.Apply("kitbag.toml", []byte(input), manifestedit.Edit{Op: manifestedit.SetInstanceHosts, Target: "c", Name: "x", Hosts: []string{"b"}})
		if !errors.Is(err, manifestedit.ErrLayout) {
			t.Fatalf("error %v for\n%s", err, input)
		}
	}
}

// TestAddInstanceWithHosts writes the list after dir and settings.
func TestAddInstanceWithHosts(t *testing.T) {
	t.Parallel()
	input := "schema = 1\n# claude\n[targets.c]\nagent = \"claude\"\n"
	off := false
	got, err := manifestedit.Apply("kitbag.toml", []byte(input), manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "x", Dir: "~/x", Settings: &off, Hosts: []string{"mac", "ci"}})
	if err != nil {
		t.Fatal(err)
	}
	want := input + "\n[targets.c.instances.x]\ndir = \"~/x\"\nsettings = false\nhosts = [\"mac\", \"ci\"]\n"
	if string(got) != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

// TestSetInstanceHostsVerifies refuses a list the manifest rejects, such as
// one on a target that a host's replacement set also declares.
func TestSetInstanceHostsVerifies(t *testing.T) {
	t.Parallel()
	input := "schema = 1\n[targets.c]\nagent = \"claude\"\n[targets.c.instances.x]\ndir = \"~/x\"\n[hosts.ci.targets.c.instances.x]\ndir = \"~/x\"\n"
	_, err := manifestedit.Apply("kitbag.toml", []byte(input), manifestedit.Edit{Op: manifestedit.SetInstanceHosts, Target: "c", Name: "x", Hosts: []string{"mac"}})
	if !errors.Is(err, manifestedit.ErrVerify) {
		t.Fatalf("error %v", err)
	}
}
