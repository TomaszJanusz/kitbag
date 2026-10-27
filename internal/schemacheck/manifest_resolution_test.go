package schemacheck_test

import (
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// undeclaredHost is resolved for every fixture besides its declared hosts.
const undeclaredHost = "undeclared-host"

// TestManifestResolution pins what every valid fixture resolves to on each
// declared host and on an undeclared one. The goldens of fixtures without
// instance hosts were recorded before that key existed, so manifests that do
// not use it keep resolving byte for byte as before.
func TestManifestResolution(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(schemasDir)
	for _, name := range examples(t, fsys, "examples/kitbag/valid") {
		t.Run(path.Base(name), func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Parse(path.Base(name), read(t, fsys, name))
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "testdata/resolution/"+strings.TrimSuffix(path.Base(name), ".toml")+".golden", resolutions(t, m))
		})
	}
}

// resolutions renders the host-resolved manifest for each host as TOML.
func resolutions(t *testing.T, m *manifest.Manifest) string {
	t.Helper()
	var out strings.Builder
	for _, host := range append(slices.Sorted(maps.Keys(m.Hosts)), undeclaredHost) {
		resolved, err := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": host})), "")
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		out.WriteString("# host " + host + "\n") //nolint:errcheck // strings.Builder never fails.
		if err = toml.NewEncoder(&out).Encode(resolved); err != nil {
			t.Fatal(err)
		}
	}
	return out.String()
}
