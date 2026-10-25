package main

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

func TestConfigDocumentation(t *testing.T) {
	t.Parallel()
	const filename = "../../docs/config.md"
	data := []byte(settingskey.Documentation())
	update, _ := newHost().LookupEnv("KITBAG_UPDATE_CONFIG")
	if update == "1" {
		if err := os.WriteFile(filename, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatal("docs/config.md is stale; run mise run docs:config")
	}
}

type configSchemaNode struct {
	Properties map[string]configSchemaNode `json:"properties"`
	Type       string                      `json:"type"`
	Enum       []string                    `json:"enum"`
}

func configSchemaLeaves(prefix string, node configSchemaNode, keys map[string]configSchemaNode) {
	for name, child := range node.Properties {
		path := prefix + name
		if child.Type == "object" {
			configSchemaLeaves(path+".", child, keys)
		} else {
			keys[path] = child
		}
	}
}

func TestConfigRegistrySchemaParity(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../schemas/config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema configSchemaNode
	if err = json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	leaves := map[string]configSchemaNode{}
	configSchemaLeaves("", schema, leaves)
	for _, key := range settingskey.Registry() {
		node, ok := leaves[key.Path]
		if !ok || node.Type != key.Type {
			t.Errorf("registry/schema drift: %s", key.Path)
		}
		if len(node.Enum) > 0 && !slices.Equal(node.Enum, key.Allowed) {
			t.Errorf("allowed values drift: %s", key.Path)
		}
		delete(leaves, key.Path)
	}
	if len(leaves) != 0 {
		t.Fatalf("schema keys without registry entries: %v", leaves)
	}
}

func TestSchemaStoreIdentifiers(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/schemastore/catalog-entries.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		URL string `json:"url"`
	}
	if err = json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	schemas := []string{"kitbag", "config"}
	if len(entries) != len(schemas) {
		t.Fatal("catalog must cover both schemas")
	}
	for i, name := range schemas {
		raw, readErr := os.ReadFile("../../schemas/" + name + ".schema.json") //nolint:gosec // G304: name comes only from the fixed two-schema list above.
		if readErr != nil {
			t.Fatal(readErr)
		}
		var schema struct {
			ID string `json:"$id"`
		}
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if entries[i].URL != schema.ID {
			t.Errorf("%s catalog URL differs from published identifier", name)
		}
	}
}
