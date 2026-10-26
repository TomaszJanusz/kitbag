package manifest_test

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// TestChecksMatchParse keeps the exported validators equal to the parser's
// grammar: every value is tried both ways.
func TestChecksMatchParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		check func(string) error
		build func(string) string
		name  string
		value string
	}{
		{manifest.CheckName, instanceName, "name", "second"},
		{manifest.CheckName, instanceName, "name dots", "claude.alt_2-x"},
		{manifest.CheckName, instanceName, "name upper", "Second"},
		{manifest.CheckName, instanceName, "name empty", ""},
		{manifest.CheckName, instanceName, "name reserved", "con"},
		{manifest.CheckName, instanceName, "name trailing dot", "a."},
		{manifest.CheckName, instanceName, "name long", strings.Repeat("a", 65)},
		{manifest.CheckHostName, hostName, "host", "Mac-Book.local"},
		{manifest.CheckHostName, hostName, "host space", "my mac"},
		{manifest.CheckHostName, hostName, "host reserved", "NUL"},
		{manifest.CheckHomePath, homeDir, "home", "~/.claude_2"},
		{manifest.CheckHomePath, homeDir, "home nested", "~/work/.codex"},
		{manifest.CheckHomePath, homeDir, "home absolute", "/home/me/.claude"},
		{manifest.CheckHomePath, homeDir, "home dotdot", "~/../x"},
		{manifest.CheckHomePath, homeDir, "home bare", "~"},
		{manifest.CheckHomePath, homeDir, "home trailing slash", "~/x/"},
		{manifest.CheckHomePath, homeDir, "home forbidden", "~/a:b"},
		{manifest.CheckHomePath, homeDir, "home backslash", `~/a\b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checked := tc.check(tc.value)
			_, parsed := manifest.Parse("kitbag.toml", []byte(tc.build(tc.value)))
			if (checked == nil) != (parsed == nil) {
				t.Fatalf("check %v, parse %v", checked, parsed)
			}
		})
	}
}

func quoted(value string) string { return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"` }

func instanceName(name string) string {
	return "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances." + quoted(name) + "]\ndir='~/x'\n"
}

func hostName(name string) string { return "schema=1\n[hosts." + quoted(name) + "]\nmode='copy'\n" }

func homeDir(dir string) string {
	return "schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.x]\ndir=" + quoted(dir) + "\n"
}
