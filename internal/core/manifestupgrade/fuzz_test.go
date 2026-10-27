package manifestupgrade_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// FuzzUpgrade asserts "refuse, or an equivalent, lossless schema-2 file":
// Prepare checks per-host equivalence itself; an accepted result must parse
// as schema 2 and keep every comment.
func FuzzUpgrade(f *testing.F) {
	for _, name := range []string{"agent-config.toml", "host-instances.toml", "comments.toml"} {
		seed, err := testdata.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(seed)
	}
	f.Add([]byte("schema=1\n[targets.c]\nagent='claude'\n[hosts.h.targets.c.instances.a]\ndir='~/a'\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		before, err := manifest.Parse("kitbag.toml", data)
		if err != nil || before.Schema != 1 {
			return
		}
		plan, err := prepare(t, data, nil)
		if err != nil {
			if !errors.Is(err, manifestupgrade.ErrRefused) {
				t.Fatalf("unexpected failure %v for\n%s", err, data)
			}
			return
		}
		after, err := manifest.Parse("kitbag.toml", plan.After)
		if err != nil || after.Schema != 2 {
			t.Fatalf("accepted result does not parse as schema 2: %v\n%s", err, plan.After)
		}
		assertLossless(t, data, plan.After)
	})
}
