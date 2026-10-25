package claude_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/claude"
)

func TestJQGoldens(t *testing.T) {
	t.Parallel()
	inputs, err := filepath.Glob("testdata/merge/*.current.json")
	if err != nil || len(inputs) == 0 {
		t.Fatalf("golden inputs: %v", err)
	}
	for _, input := range inputs {
		name := strings.TrimSuffix(input, ".current.json")
		t.Run(filepath.Base(name), func(t *testing.T) {
			t.Parallel()
			current := read(t, input)
			fragment := read(t, name+".fragment.json")
			originalCurrent, originalFragment := bytes.Clone(current), bytes.Clone(fragment)
			got, mergeErr := (claude.Merger{Repository: "/fixture/config repo"}).Merge(current, fragment)
			if !bytes.Equal(current, originalCurrent) || !bytes.Equal(fragment, originalFragment) {
				t.Fatal("mutated merge input")
			}
			checkGolden(t, name, fragment, got, mergeErr)
		})
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(os.DirFS("testdata/merge"), filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRejectUnsafeSettings(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{`{} {}`, `{}`},
		{`{}`, `null`},
		{`{}`, `[]`},
		{`{}`, `{bad`},
		{`{"hooks":[]}`, `{}`},
		{`{}`, `{"hooks":{"Stop":{}}}`},
		{`{}`, `{"hooks":{"Stop":[{"hooks":[{"command":12}]}]}}`},
		{`{}`, `{"hooks":{"Stop":[{"hooks":[{"type":"prompt","command":"bad"}]}]}}`},
	} {
		if _, err := (claude.Merger{}).Merge([]byte(pair[0]), []byte(pair[1])); err == nil {
			t.Fatalf("accepted unsafe settings: %v", pair)
		}
	}
}

func checkGolden(t *testing.T, name string, fragment, got []byte, mergeErr error) {
	t.Helper()
	if _, statErr := os.Stat(name + ".error.txt"); statErr == nil {
		if mergeErr == nil || got != nil {
			t.Fatalf("unsafe merge accepted: %s", got)
		}
		return
	}
	if mergeErr != nil {
		t.Fatal(mergeErr)
	}
	if want := read(t, name+".want.json"); !bytes.Equal(got, want) {
		t.Fatalf("jq parity:\ngot:\n%s\nwant:\n%s", got, want)
	}
	again, mergeErr := (claude.Merger{Repository: "/fixture/config repo"}).Merge(got, fragment)
	if mergeErr != nil || !bytes.Equal(got, again) {
		t.Fatalf("merge not idempotent: %s, %v", again, mergeErr)
	}
	checkLegacy(t, name, got)
}

func checkLegacy(t *testing.T, name string, got []byte) {
	t.Helper()
	// The legacy rendering is exactly what agent-config-sync's jq produced.
	if legacy, err := (claude.Merger{}).Legacy(got); err != nil || !bytes.Equal(legacy, read(t, name+".legacy.json")) {
		t.Fatalf("legacy rendering:\ngot:\n%s\nwant:\n%s (%v)", legacy, read(t, name+".legacy.json"), err)
	}
	unmerged, err := (claude.Merger{}).Unmerge(got)
	if err != nil || bytes.Contains(unmerged, []byte(" "+claude.HookMarker+`"`)) {
		t.Fatalf("unmerge kept managed hooks: %s, %v", unmerged, err)
	}
}

func TestUnmerge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, current, want string }{
		{"empty", ``, ``},
		{"unowned unchanged", `{"b":1,  "a":2}`, `{"b":1,  "a":2}`},
		{"foreign hooks kept", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x"}]}]}}`, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x"}]}]}}`},
		{"owned removed", `{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"x # kitbag-managed"}]}],"Start":[{"hooks":[{"type":"command","command":"y"}]}]}}`, "{\n  \"theme\": \"dark\",\n  \"hooks\": {\n    \"Start\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"y\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"},
		{"legacy removed with empty hooks", `{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"x # agent-config-managed"}]}]}}`, "{\n  \"theme\": \"dark\"\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := (claude.Merger{}).Unmerge([]byte(tc.current))
			if err != nil || string(got) != tc.want {
				t.Fatalf("Unmerge = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, bad := range []string{`[]`, `{"hooks":[]}`, `{bad`} {
		if _, err := (claude.Merger{}).Unmerge([]byte(bad)); err == nil {
			t.Fatalf("Unmerge accepted %s", bad)
		}
	}
	if _, err := (claude.Merger{}).Legacy([]byte(`[]`)); err == nil {
		t.Fatal("Legacy accepted a non-object")
	}
}
