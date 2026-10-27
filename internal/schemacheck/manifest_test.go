package schemacheck_test

import (
	"errors"
	"flag"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Flags live in TestMain rather than package variables (gochecknoglobals).
func TestMain(m *testing.M) {
	flag.Bool("update", false, "rewrite manifest diagnostic golden files")
	flag.Parse()
	os.Exit(m.Run())
}

// TestManifestSchemaParity runs both independent readers over the SAME fixtures.
// Expectations identify the intended rule, so an unrelated rejection cannot pass.
// config.toml has its own schema and is outside pkg/manifest's API.
func TestManifestSchemaParity(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(schemasDir)
	schema := compile(t, fsys, "kitbag.schema.json")
	for _, status := range []string{"valid", "invalid"} {
		for _, name := range examples(t, fsys, path.Join("examples/kitbag/v1", status)) {
			t.Run(status+"/"+path.Base(name), func(t *testing.T) {
				t.Parallel()
				raw := read(t, fsys, name)
				m, err := manifest.Parse(path.Base(name), raw)
				oracle := validate(schema, raw)
				if (err == nil) != (oracle == nil) {
					t.Fatalf("parser/schema disagree: parser=%v; schema=%v", err, oracle)
				}
				if status == "valid" {
					if err != nil || m == nil {
						t.Fatalf("valid fixture: %v", err)
					}
					return
				}
				assertManifestError(t, name, raw, err)
			})
		}
	}
}

func assertManifestError(t *testing.T, name string, raw []byte, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid fixture accepted")
	}
	want := expectation(t, name, raw)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not mention intended rule %q", err, want)
	}
	var diagnostic *manifest.Error
	if !errors.As(err, &diagnostic) || diagnostic.Line < 1 || diagnostic.Column < 1 {
		t.Fatalf("missing position: %v", err)
	}
	golden(t, "testdata/manifest/"+path.Base(path.Dir(path.Dir(name)))+"/"+path.Base(name)+".golden", err.Error()+"\n")
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	if flag.Lookup("update").Value.String() == "true" {
		if err := os.MkdirAll(path.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := read(t, os.DirFS("."), name)
	if got != string(want) {
		t.Fatalf("diagnostic golden mismatch:\ngot: %s\nwant: %s", got, want)
	}
}

// Mutate every field in the all-keys fixture to wrong scalar types, and add
// unknown keys at every object level, using the schema as an independent oracle.
// This guards optional fields and nested structures beyond the curated examples.
func TestManifestSchemaMutations(t *testing.T) {
	t.Parallel()
	fsys := os.DirFS(schemasDir)
	schema := compile(t, fsys, "kitbag.schema.json")
	raw := read(t, fsys, "examples/kitbag/v1/valid/full.toml")
	for i, mutation := range manifestMutations(t, raw) {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			oracle := validate(schema, mutation)
			_, err := manifest.Parse("mutation.toml", mutation)
			if oracle == nil {
				t.Fatal("mutation did not invalidate the schema")
			}
			if err == nil {
				t.Fatalf("parser accepted schema-invalid mutation:\n%s", mutation)
			}
		})
	}
}
