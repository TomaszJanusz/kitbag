package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestFakeContract(t *testing.T) {
	t.Parallel()
	runContract(t, newFakeSubject)
}

func newFakeSubject(t *testing.T, env map[string]string) subject {
	t.Helper()
	h := fake.New(fake.WithEnv(env))
	root, other := filepath.FromSlash("/work"), filepath.FromSlash("/other")
	mustNil(t, "mkdir root", h.MkdirAll(root, 0o700))
	mustNil(t, "mkdir other", h.MkdirAll(other, 0o700))
	mustNil(t, "mount other", h.Mount(other))
	return subject{
		host:        h,
		root:        root,
		permissions: true,
		otherDevice: other,
		nonTerminal: 1,
		opened:      h.Opened,
	}
}
