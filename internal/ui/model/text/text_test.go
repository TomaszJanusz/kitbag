package text

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

// declaredKeys parses this package's source so a new constant cannot be
// added without its English default.
func declaredKeys(t *testing.T) map[Key]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "text.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[Key]string)
	ast.Inspect(file, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, typed := value.Type.(*ast.Ident); !typed || ident.Name != "Key" {
			return false
		}
		literal, ok := value.Values[0].(*ast.BasicLit)
		if !ok {
			t.Fatalf("%s is not a literal key", value.Names[0].Name)
		}
		keys[Key(strings.Trim(literal.Value, `"`))] = value.Names[0].Name
		return false
	})
	return keys
}

func TestEveryKeyHasAnEnglishDefault(t *testing.T) {
	t.Parallel()
	catalog, keys := English(), declaredKeys(t)
	if len(keys) < 100 {
		t.Fatalf("parsed only %d keys", len(keys))
	}
	for key, name := range keys {
		if strings.TrimSpace(catalog[key]) == "" {
			t.Errorf("%s (%q) has no English default", name, key)
		}
	}
	for _, setting := range settingskey.Registry() {
		key := SettingDescription(setting.Path)
		if strings.TrimSpace(catalog[key]) == "" || keys[key] != "" {
			t.Errorf("%s description missing from the catalog", setting.Path)
		}
		keys[key] = setting.Path
	}
	for key := range catalog {
		if keys[key] == "" {
			t.Errorf("catalog entry %q has no declared key", key)
		}
	}
}

func TestTemplatesUseOnlyStringVerbs(t *testing.T) {
	t.Parallel()
	verb := regexp.MustCompile(`%.`)
	for key, template := range English() {
		for _, found := range verb.FindAllString(template, -1) {
			if found != "%s" && found != "%%" {
				t.Errorf("%s uses %q; arguments are rendered messages", key, found)
			}
		}
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	catalog := make(Catalog)
	catalog["outer"], catalog["inner"], catalog["flat"] = "a %s b", "<%s>", "100%%"
	if got := catalog.Render(New("outer", New("inner", Plain("x%sy")))); got != "a <x%sy> b" {
		t.Fatalf("nested=%q", got)
	}
	if got := catalog.Render(New("flat")); got != "100%" {
		t.Fatalf("flat=%q", got)
	}
	if got := catalog.Render(New("missing", Plain("x"))); got != "missing" {
		t.Fatalf("missing=%q", got)
	}
	if got := English().Render(New(StatusSyncing, New(PhasePull))); got != "Syncing: pulling the repository" {
		t.Fatalf("english=%q", got)
	}
}
