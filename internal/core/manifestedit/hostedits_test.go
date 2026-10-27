package manifestedit_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
)

const schema2Head = "schema = 2\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\"\n"

// TestHostEdits covers the edits of the schema upgrade on schema-2 inputs,
// where every intermediate result is valid on its own.
func TestHostEdits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input, want string
		edits             []manifestedit.Edit
	}{
		{
			"schema and minimum added after it", "schema = 1 # v\n", "schema = 2 # v\nmin_tool_version = \"0.1.4\"\n",
			[]manifestedit.Edit{{Op: manifestedit.SetSchema, Schema: 2}, {Op: manifestedit.SetMinToolVersion, Version: "0.1.4"}},
		},
		{
			"minimum replaced", "schema = 2\nmin_tool_version = '0.1.0' # floor\n", "schema = 2\nmin_tool_version = \"0.1.4\" # floor\n",
			[]manifestedit.Edit{{Op: manifestedit.SetMinToolVersion, Version: "0.1.4"}},
		},
		{
			"patch after the host's tables", schema2Head + "\n[hosts.h]\nmode = \"copy\"\n\n[publish]\nmode = \"pr\"\n",
			schema2Head + "\n[hosts.h]\nmode = \"copy\"\n\n[hosts.h.targets.c.instances.x]\nenabled = false\nmode = \"link\"\nsettings = true\n\n[publish]\nmode = \"pr\"\n",
			[]manifestedit.Edit{{Op: manifestedit.AddHostPatch, Host: "h", Target: "c", Name: "x", Enabled: off(), Mode: "link", Settings: on()}},
		},
		{
			"patch removed with empty parents, comments kept", schema2Head + "\n[hosts.h]\n# note\n[hosts.h.targets.c.instances.x]\nenabled = false # why\n",
			schema2Head + "\n# note\n# why\n",
			[]manifestedit.Edit{{Op: manifestedit.RemoveHostInstance, Host: "h", Target: "c", Name: "x"}},
		},
		{
			"instance added with hosts", "schema = 2\n[targets.c]\nagent = \"claude\"\n", "schema = 2\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.y]\ndir = \"~/y\"\nhosts = [\"a\", \"b\"]\n",
			[]manifestedit.Edit{{Op: manifestedit.AddInstance, Target: "c", Name: "y", Dir: "~/y", Hosts: []string{"a", "b"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifestedit.Apply("kitbag.toml", []byte(tc.input), tc.edits...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestMoveHostInstance(t *testing.T) {
	t.Parallel()
	input := "schema = 1\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\"\n\n# only on h\n[hosts.h.targets.c.instances.y]\ndir = \"~/y\" # y dir\n[hosts.h.targets.c.instances.y.roots]\nskills = \"~/s\"\n# after\n"
	want := "schema = 2\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\"\n\n# only on h\n[targets.c.instances.y]\ndir = \"~/y\" # y dir\nhosts = [\"h\"]\n[targets.c.instances.y.roots]\nskills = \"~/s\"\n\n# after\n\n[hosts.h.targets.c.instances.x]\nenabled = false\n"
	got, err := manifestedit.Apply("kitbag.toml", []byte(input),
		manifestedit.Edit{Op: manifestedit.SetSchema, Schema: 2},
		manifestedit.Edit{Op: manifestedit.AddHostPatch, Host: "h", Target: "c", Name: "x", Enabled: off()},
		manifestedit.Edit{Op: manifestedit.MoveHostInstance, Host: "h", Target: "c", Name: "y", Hosts: []string{"h"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestHostEditRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		want        error
		name, input string
		edit        manifestedit.Edit
	}{
		{manifestedit.ErrLayout, "inline host instances", "schema = 2\n[targets.c]\nagent = \"claude\"\n[targets.c.instances.x]\ndir = \"~/x\"\n[hosts.h.targets.c]\ninstances = { x = { enabled = false } }\n", manifestedit.Edit{Op: manifestedit.RemoveHostInstance, Host: "h", Target: "c", Name: "x"}},
		{manifestedit.ErrNotFound, "missing host instance", schema2Head, manifestedit.Edit{Op: manifestedit.RemoveHostInstance, Host: "h", Target: "c", Name: "x"}},
		{manifestedit.ErrExists, "patch twice", schema2Head + "[hosts.h.targets.c.instances.x]\nenabled = false\n", manifestedit.Edit{Op: manifestedit.AddHostPatch, Host: "h", Target: "c", Name: "x", Enabled: off()}},
		{manifestedit.ErrLayout, "dotted schema", "schema = 2\n[targets]\nc.agent = \"claude\"\n", manifestedit.Edit{Op: manifestedit.AddInstance, Target: "c", Name: "x", Dir: "~/x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := manifestedit.Apply("kitbag.toml", []byte(tc.input), tc.edit); !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("got %q, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// TestSetInstanceHosts rewrites, adds and deletes an instance's hosts list
// and changes nothing else in the file.
func TestSetInstanceHosts(t *testing.T) {
	t.Parallel()
	head := "# top\nschema = 2\n[targets.c]\nagent = \"claude\"\n\n[targets.c.instances.x]\ndir = \"~/x\" # dir\n"
	cases := []struct {
		name, input, want string
		hosts             []string
	}{
		{"added after dir", head + "settings = false\n", head + "hosts = [\"mac\"]\nsettings = false\n", []string{"mac"}},
		{"rewritten in place", head + "hosts = ['a'] # only a\nsettings = false\n", head + "hosts = [\"a\", \"b\"] # only a\nsettings = false\n", []string{"a", "b"}},
		{"deleted, comment kept", head + "hosts = [\"a\"] # only a\n\n[publish]\nmode = \"direct\"\n", head + "# only a\n\n[publish]\nmode = \"direct\"\n", nil},
		{"deleted at end of file without newline", head + "hosts = [\"a\"]", head, nil},
		{"absent and all hosts is a no-op", head, head, nil},
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
	head := "schema = 2\n[targets.c]\nagent = \"claude\"\n[targets.c.instances.x]\ndir = \"~/x\"\n"
	for _, input := range []string{
		head + "hosts = [\n  \"a\",\n]\n",
		head + "hosts = [\"a\", # first\n \"b\"]\n",
		"schema = 2\n[targets.c]\nagent = \"claude\"\ninstances.x = { dir = \"~/x\" }\n",
	} {
		_, err := manifestedit.Apply("kitbag.toml", []byte(input), manifestedit.Edit{Op: manifestedit.SetInstanceHosts, Target: "c", Name: "x", Hosts: []string{"b"}})
		if !errors.Is(err, manifestedit.ErrLayout) {
			t.Fatalf("error %v for\n%s", err, input)
		}
	}
}
