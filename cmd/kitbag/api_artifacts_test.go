package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/apigen"
)

func TestAPIArtifacts(t *testing.T) {
	t.Parallel()
	schema, err := os.ReadFile("../../docs/api/api-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	reference, tests, err := apigen.Generate(schema)
	if err != nil {
		t.Fatal(err)
	}
	update, _ := newHost().LookupEnv("KITBAG_UPDATE_API")
	for path, want := range map[string][]byte{"../../docs/api/api-v1.md": reference, "api_contract_generated_test.go": tests} {
		checkAPIArtifact(t, path, want, update == "1")
	}
}

func checkAPIArtifact(t *testing.T, path string, want []byte, update bool) {
	t.Helper()
	if update {
		if err := os.WriteFile(path, want, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path) //nolint:gosec // G304: paths are the two fixed generated artifacts above.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; run mise run docs:api", path)
	}
}
