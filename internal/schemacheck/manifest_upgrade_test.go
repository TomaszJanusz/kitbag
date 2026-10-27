package schemacheck_test

import (
	"errors"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// refusedUpgrade is the schema-1 example the upgrade must refuse: its runner02
// set names ~/.claude "only" while the target calls it main.
const refusedUpgrade = "full.toml"

// TestManifestUpgradeExamples upgrades every valid schema-1 example. Prepare
// itself proves per-host equivalence; the result must keep every comment,
// satisfy the reader and kitbag.schema.json, and fail kitbag.v1.schema.json.
func TestManifestUpgradeExamples(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(schemasDir)
	current, v1 := compile(t, fsys, "kitbag.schema.json"), compile(t, fsys, "kitbag.v1.schema.json")
	kinds := discover.Table{{Agent: "claude", DefaultDir: "~/.claude"}, {Agent: "codex", DefaultDir: "~/.codex"}, {Agent: "generic"}}
	for _, name := range examples(t, fsys, "examples/kitbag/v1/valid") {
		t.Run(path.Base(name), func(t *testing.T) {
			t.Parallel()
			raw := read(t, fsys, name)
			plan, err := manifestupgrade.Prepare(manifestupgrade.Input{Filename: path.Base(name), HostName: "test", Manifest: raw, Kinds: kinds})
			if path.Base(name) == refusedUpgrade {
				if !errors.Is(err, manifestupgrade.ErrRefused) {
					t.Fatalf("%v, want a refusal", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m, parseErr := manifest.Parse(path.Base(name), plan.After); parseErr != nil || m.Schema != 2 {
				t.Fatalf("upgraded: %v", parseErr)
			}
			if err = validate(current, plan.After); err != nil {
				t.Fatalf("kitbag.schema.json rejects the upgrade: %v", err)
			}
			if validate(v1, plan.After) == nil {
				t.Fatal("kitbag.v1.schema.json accepts the upgrade")
			}
			assertComments(t, raw, plan.After)
		})
	}
}

var comment = regexp.MustCompile(`#.*`)

func assertComments(t *testing.T, before, after []byte) {
	t.Helper()
	count := map[string]int{}
	for _, c := range comment.FindAll(before, -1) {
		count[strings.TrimSpace(string(c))]++
	}
	for _, c := range comment.FindAll(after, -1) {
		count[strings.TrimSpace(string(c))]--
	}
	for c, missing := range count {
		if missing != 0 {
			t.Fatalf("comment %q: %d missing", c, missing)
		}
	}
}
