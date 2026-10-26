//go:build darwin || linux

package unix

import (
	"os"
	"path/filepath"
	"testing"
)

// homebrewTree builds <root>/Cellar/kitbag/0.1.0/bin/kitbag with the opt and
// bin links brew creates, and returns the resolved prefix.
func homebrewTree(t *testing.T, links ...string) string {
	t.Helper()
	return homebrewLayout(t, "bin", links...)
}

// homebrewLayout also models the formula: libexec/kitbag with bin/kitbag
// linking to it inside the keg, then the prefix bin and opt links.
func homebrewLayout(t *testing.T, directory string, links ...string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keg := filepath.Join(root, "Cellar", "kitbag", "0.1.0")
	for _, dir := range []string{filepath.Join(keg, directory), filepath.Join(keg, "bin"), filepath.Join(root, "opt"), filepath.Join(root, "bin")} {
		if err = os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(keg, directory, "kitbag"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if directory == "libexec" {
		if err = os.Symlink("../libexec/kitbag", filepath.Join(keg, "bin", "kitbag")); err != nil {
			t.Fatal(err)
		}
	}
	targets := map[string][2]string{
		"opt": {"../Cellar/kitbag/0.1.0", filepath.Join(root, "opt", "kitbag")},
		"bin": {"../Cellar/kitbag/0.1.0/bin/kitbag", filepath.Join(root, "bin", "kitbag")},
	}
	for _, name := range links {
		if err = os.Symlink(targets[name][0], targets[name][1]); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestStableExecutable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		links      []string
	}{
		{"prefers-opt", "opt/kitbag/bin/kitbag", []string{"opt", "bin"}},
		{"falls-back-to-bin", "bin/kitbag", []string{"bin"}},
		{"keeps-keg-without-links", "Cellar/kitbag/0.1.0/bin/kitbag", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := homebrewTree(t, tc.links...)
			got := stableExecutable(filepath.Join(root, "Cellar", "kitbag", "0.1.0", "bin", "kitbag"))
			if want := filepath.Join(root, tc.want); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestStableExecutableRejectsOtherTargets(t *testing.T) {
	t.Parallel()
	root := homebrewTree(t)
	// opt points at another version: using it would silently switch binaries.
	newer := filepath.Join(root, "Cellar", "kitbag", "0.2.0", "bin")
	if err := os.MkdirAll(newer, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newer, "kitbag"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../Cellar/kitbag/0.2.0", filepath.Join(root, "opt", "kitbag")); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "Cellar", "kitbag", "0.1.0", "bin", "kitbag")
	for _, path := range []string{old, "/usr/local/bin/kitbag", "/Cellar/kitbag", filepath.Join(root, "go", "bin", "kitbag")} {
		want := path
		if got := stableExecutable(path); got != want {
			t.Fatalf("stableExecutable(%s) = %s", path, got)
		}
	}
}

func TestStableExecutableLibexec(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		links      []string
	}{
		{"prefers-opt", "opt/kitbag/libexec/kitbag", []string{"opt", "bin"}},
		{"falls-back-to-bin", "bin/kitbag", []string{"bin"}},
		{"keeps-keg-without-links", "Cellar/kitbag/0.1.0/libexec/kitbag", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := homebrewLayout(t, "libexec", tc.links...)
			resolved := filepath.Join(root, "Cellar", "kitbag", "0.1.0", "libexec", "kitbag")
			if got, want := stableExecutable(resolved), filepath.Join(root, tc.want); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestStableLibexecRejectsOtherTargets(t *testing.T) {
	t.Parallel()
	root := homebrewLayout(t, "libexec")
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "libexec"), 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(other, "libexec", "kitbag")
	if err := os.WriteFile(binary, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"opt/kitbag": other, "bin/kitbag": binary} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	resolved := filepath.Join(root, "Cellar", "kitbag", "0.1.0", "libexec", "kitbag")
	if got := stableExecutable(resolved); got != resolved {
		t.Fatalf("selected a different executable: %s", got)
	}
}
