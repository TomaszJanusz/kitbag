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
}
