package instructions_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
)

func declare(t *testing.T, name string) instructions.Result {
	t.Helper()
	for _, c := range goldenCases() {
		if c.name == name {
			got, err := instructions.Declare(fixtureHost(t), c.request())
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
	}
	t.Fatalf("no golden case %s", name)
	return instructions.Result{}
}

// TestLegacyCompatibility compares with testdata/legacy, written by running
// agent-config-sync on the same fixture inputs (its repository path mapped to
// /repo). Only the first line, the marker, may differ.
func TestLegacyCompatibility(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"import-host", "import-no-host", "concat-host", "concat-no-host"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			legacy, err := fs.ReadFile(os.DirFS(filepath.Join("testdata", "legacy")), name+".md")
			if err != nil {
				t.Fatal(err)
			}
			marker, rest, _ := bytes.Cut(legacy, []byte("\n"))
			if !bytes.HasPrefix(marker, []byte("<!-- Managed by agent-config-sync (")) {
				t.Fatalf("legacy fixture lacks its marker: %q", marker)
			}
			want := append([]byte(instructions.Marker+"\n"), rest...)
			declared := declare(t, name).Target
			if got := declared.Content; !bytes.Equal(got, want) {
				t.Fatalf("not byte-identical to agent-config-sync:\ngot:  %q\nwant: %q", got, want)
			}
			// The legacy rendering is the script's exact output for this clone.
			if !bytes.Equal(declared.Legacy, legacy) || string(marker) != instructions.LegacyMarker("/repo") {
				t.Fatalf("legacy rendering differs:\ngot:  %q\nwant: %q", declared.Legacy, legacy)
			}
		})
	}
}

func TestMarker(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"import-host", "concat-override", "concat-no-host"} {
		content := declare(t, name).Target.Content
		first, _, _ := bytes.Cut(content, []byte("\n"))
		if string(first) != instructions.Marker || !instructions.Managed(content) {
			t.Fatalf("%s: marker is not the first line: %q", name, first)
		}
		if bytes.Count(content, []byte(instructions.Marker)) != 1 {
			t.Fatalf("%s: marker repeated", name)
		}
	}
	legacy, err := fs.ReadFile(os.DirFS(filepath.Join("testdata", "legacy")), "import-host.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range [][]byte{nil, []byte("# Hand-written\n"), legacy, []byte(instructions.Marker)} {
		if instructions.Managed(content) {
			t.Fatalf("unmanaged content recognized as managed: %q", content)
		}
	}
}

// TestIdempotent renders again after writing the output to its destination:
// the destination is never an input, so the declaration is unchanged.
func TestIdempotent(t *testing.T) {
	t.Parallel()
	c := goldenCases()[3]
	h := fixtureHost(t)
	first, err := instructions.Declare(h, c.request())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, h, first.Target.Path, first.Target.Content)
	second, err := instructions.Declare(h, c.request())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Target.Content, second.Target.Content) || !slices.Equal(first.Warnings, second.Warnings) {
		t.Fatal("second render differs")
	}
}

// TestDeterministic varies map contents and repeats renders: the per-agent map
// is only looked up, never iterated, and owners are copied, not aliased.
func TestDeterministic(t *testing.T) {
	t.Parallel()
	for _, c := range []goldenCase{goldenCases()[0], goldenCases()[5]} {
		want := declare(t, c.name)
		for i := range 20 {
			req := c.request()
			for _, kind := range []string{"claude", "codex", "cursor", "opencode", "copilot", "windsurf"} {
				if kind != c.agent {
					req.Instructions.PerAgent[kind] = "agents/" + kind + ".md"
				}
			}
			got, err := instructions.Declare(fixtureHost(t), req)
			if err != nil {
				t.Fatal(err)
			}
			req.Owners[0].AgentInstance = "mutated"
			if !bytes.Equal(got.Target.Content, want.Target.Content) || got.Target.Owners[0].AgentInstance != c.agent+":main" {
				t.Fatalf("%s: render %d differs", c.name, i)
			}
		}
	}
}

func TestChoose(t *testing.T) {
	t.Parallel()
	all := []instructions.Strategy{instructions.Import, instructions.Concat}
	for _, c := range []struct {
		render, want string
		fallback     instructions.Strategy
		supported    []instructions.Strategy
	}{
		{render: "", fallback: instructions.Import, supported: all, want: "import"},
		{render: "concat", fallback: instructions.Import, supported: all, want: "concat"},
		{render: "", fallback: instructions.Link, supported: []instructions.Strategy{instructions.Link}, want: "link"},
		{render: "import", fallback: instructions.Concat, supported: []instructions.Strategy{instructions.Concat}},
		{render: "unknown", fallback: instructions.Concat, supported: all},
	} {
		got, err := instructions.Choose(c.render, c.fallback, c.supported...)
		if string(got) != c.want || (err == nil) != (c.want != "") {
			t.Fatalf("Choose(%q, %q) = %q, %v; want %q", c.render, c.fallback, got, err, c.want)
		}
	}
}

func TestDeclareRejectsInvalidRequests(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*instructions.Request){
		"no instructions": func(r *instructions.Request) { r.Instructions = nil },
		"no source":       func(r *instructions.Request) { r.Instructions.Source = "" },
		"template":        func(r *instructions.Request) { r.Instructions.Hosts = "hosts/{host}.md" },
		"no agent":        func(r *instructions.Request) { r.Agent = "" },
		"relative repo":   func(r *instructions.Request) { r.Repository = "repo" },
		"unclean path":    func(r *instructions.Request) { r.Path = "/home/user/../x" },
		"strategy":        func(r *instructions.Request) { r.Strategy = "generated" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := goldenCases()[0].request()
			mutate(&req)
			if _, err := instructions.Declare(fixtureHost(t), req); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}
