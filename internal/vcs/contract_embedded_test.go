package vcs_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded"
)

// TestEmbeddedContract runs exactly the same cases as TestSystemContract.
func TestEmbeddedContract(t *testing.T) {
	t.Parallel()
	runContract(t, newEmbeddedSubject)
}

func newEmbeddedSubject(t *testing.T) subject {
	t.Helper()
	sub := newSystemSubject(t)
	sub.backend = embedded.New(embedded.Options{Env: fake.New(fake.WithEnv(map[string]string{"HOME": t.TempDir()})), Identity: vcs.Identity{Name: "KitBag Test", Email: "test@kitbag.invalid"}, NoSystemConfig: true})
	return sub
}
