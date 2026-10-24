package schemacheck_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemasDir   = "../../schemas"
	docsDir      = "../../docs"
	metaSchema   = "https://json-schema.org/draft/2020-12/schema"
	idPrefix     = "https://kitbag.sh/schemas/"
	expectPrefix = "# expect: "
	exampleMark  = "<!-- example: "
	fence        = "```"
)

// kind pairs one schema file with the directory of its TOML examples.
type kind struct {
	name   string
	schema string
}

func kinds() []kind {
	return []kind{
		{name: "kitbag", schema: "kitbag.schema.json"},
		{name: "config", schema: "config.schema.json"},
	}
}

func TestValidExamples(t *testing.T) {
	t.Parallel()

	for _, k := range kinds() {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()

			fsys := os.DirFS(schemasDir)
			schema := compile(t, fsys, k.schema)
			for _, name := range examples(t, fsys, path.Join("examples", k.name, "valid")) {
				t.Run(path.Base(name), func(t *testing.T) {
					t.Parallel()

					if err := validate(schema, read(t, fsys, name)); err != nil {
						t.Errorf("%s: expected a valid example, got:\n%v", name, err)
					}
				})
			}
		})
	}
}

func TestInvalidExamples(t *testing.T) {
	t.Parallel()

	for _, k := range kinds() {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()

			fsys := os.DirFS(schemasDir)
			schema := compile(t, fsys, k.schema)
			for _, name := range examples(t, fsys, path.Join("examples", k.name, "invalid")) {
				t.Run(path.Base(name), func(t *testing.T) {
					t.Parallel()

					raw := read(t, fsys, name)
					want := expectation(t, name, raw)
					err := validate(schema, raw)
					if err == nil {
						t.Fatalf("%s: expected validation to fail with %q", name, want)
					}
					if !strings.Contains(err.Error(), want) {
						t.Errorf("%s: error does not mention %q:\n%v", name, want, err)
					}
				})
			}
		})
	}
}

// TestSchemaIdentity pins the draft and the stable $id of every schema file.
func TestSchemaIdentity(t *testing.T) {
	t.Parallel()

	for _, k := range kinds() {
		t.Run(k.name, func(t *testing.T) {
			t.Parallel()

			var header struct {
				Schema string `json:"$schema"`
				ID     string `json:"$id"`
			}
			raw := read(t, os.DirFS(schemasDir), k.schema)
			if err := json.Unmarshal(raw, &header); err != nil {
				t.Fatalf("%s: %v", k.schema, err)
			}
			if header.Schema != metaSchema {
				t.Errorf("%s: $schema = %q, want %q", k.schema, header.Schema, metaSchema)
			}
			if want := idPrefix + k.schema; header.ID != want {
				t.Errorf("%s: $id = %q, want %q", k.schema, header.ID, want)
			}
		})
	}
}

// TestDocumentedExamples keeps the TOML blocks quoted in docs/manifest.md
// byte-identical to the validated fixtures they are marked with.
func TestDocumentedExamples(t *testing.T) {
	t.Parallel()

	doc := string(read(t, os.DirFS(docsDir), "manifest.md"))
	blocks := fencedBlocks(doc)
	if len(blocks) == 0 {
		t.Fatalf("docs/manifest.md has no %q markers", exampleMark)
	}
	for file, block := range blocks {
		if !strings.HasPrefix(file, "schemas/") {
			t.Errorf("marker %q must point below schemas/", file)
			continue
		}
		want := string(read(t, os.DirFS(schemasDir), strings.TrimPrefix(file, "schemas/")))
		if block != want {
			t.Errorf("docs/manifest.md block for %s differs from the fixture:\n%s", file, block)
		}
	}
}

// compile loads one schema file through the library so that a malformed
// schema fails the gate as loudly as a malformed example.
func compile(t *testing.T, fsys fs.FS, name string) *jsonschema.Schema {
	t.Helper()

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(read(t, fsys, name)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	compiler := jsonschema.NewCompiler()
	url := idPrefix + name
	if addErr := compiler.AddResource(url, doc); addErr != nil {
		t.Fatalf("%s: %v", name, addErr)
	}
	schema, compileErr := compiler.Compile(url)
	if compileErr != nil {
		t.Fatalf("%s: %v", name, compileErr)
	}
	return schema
}

// validate decodes TOML into the JSON data model and validates it.
func validate(schema *jsonschema.Schema, raw []byte) error {
	var tree map[string]any
	if _, err := toml.Decode(string(raw), &tree); err != nil {
		return err
	}
	encoded, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	return schema.Validate(instance)
}

func examples(t *testing.T, fsys fs.FS, dir string) []string {
	t.Helper()

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			names = append(names, path.Join(dir, entry.Name()))
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s: no .toml examples", dir)
	}
	return names
}

func read(t *testing.T, fsys fs.FS, name string) []byte {
	t.Helper()

	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return raw
}

// expectation returns the error fragment an invalid example announces on its
// first line, so that each example fails for its intended reason.
func expectation(t *testing.T, name string, raw []byte) string {
	t.Helper()

	first, _, _ := strings.Cut(string(raw), "\n")
	want, ok := strings.CutPrefix(first, expectPrefix)
	if !ok || want == "" {
		t.Fatalf("%s: first line must be %q followed by the expected error", name, expectPrefix)
	}
	return want
}

// fencedBlocks maps each "<!-- example: path -->" marker in a Markdown
// document to the content of the fenced code block that follows it.
func fencedBlocks(doc string) map[string]string {
	blocks := map[string]string{}
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(line, exampleMark)
		if !ok {
			continue
		}
		file := strings.TrimSpace(strings.TrimSuffix(rest, "-->"))
		blocks[file] = blockAfter(lines[i+1:])
	}
	return blocks
}

func blockAfter(lines []string) string {
	var body []string
	open := false
	for _, line := range lines {
		switch {
		case !open && strings.HasPrefix(line, fence):
			open = true
		case open && line == fence:
			return strings.Join(body, "\n") + "\n"
		case open:
			body = append(body, line)
		}
	}
	return ""
}
