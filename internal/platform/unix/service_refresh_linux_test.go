package unix

import (
	"path/filepath"
	"testing"
)

func TestSystemdHomebrewUsesStableExecutable(t *testing.T) {
	t.Parallel()
	root := homebrewLayout(t, "libexec", "opt", "bin")
	h := New(WithEnv(func(key string) (string, bool) { return map[string]string{"HOME": root}[key], key == "HOME" }), WithExecutable(func() (string, error) {
		return filepath.Join(root, "Cellar", "kitbag", "0.1.0", "libexec", "kitbag"), nil
	}))
	layout, err := h.ServiceLayout()
	if err != nil || layout.Binary != filepath.Join(root, "opt", "kitbag", "libexec", "kitbag") {
		t.Fatal(layout, err)
	}
}
