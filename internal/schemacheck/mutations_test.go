package schemacheck_test

import (
	"bytes"
	"maps"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
)

func manifestMutations(t *testing.T, raw []byte) [][]byte {
	t.Helper()
	var tree map[string]any
	if _, err := toml.Decode(string(raw), &tree); err != nil {
		t.Fatal(err)
	}
	var results [][]byte
	mutateTable(t, tree, tree, &results)
	return results
}

func encodeMutation(t *testing.T, root map[string]any, results *[][]byte) {
	t.Helper()
	var output bytes.Buffer
	if err := toml.NewEncoder(&output).Encode(root); err != nil {
		t.Fatal(err)
	}
	*results = append(*results, slices.Clone(output.Bytes()))
}

func mutateTable(t *testing.T, root, table map[string]any, results *[][]byte) {
	t.Helper()
	table["kitbag_unknown_key"] = true
	encodeMutation(t, root, results)
	delete(table, "kitbag_unknown_key")
	for _, key := range slices.Sorted(maps.Keys(table)) {
		value := table[key]
		// No schema-v1 field accepts integer 42, except schema, which is const 1.
		table[key] = int64(42)
		encodeMutation(t, root, results)
		table[key] = value
		descendMutation(t, root, value, results)
	}
}

func descendMutation(t *testing.T, root map[string]any, value any, results *[][]byte) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		mutateTable(t, root, node, results)
	case []map[string]any:
		for _, table := range node {
			mutateTable(t, root, table, results)
		}
	}
}
