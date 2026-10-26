package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestParseRejectsNewerSchemaFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		found      int64
		line       int
	}{
		{"next schema with unknown keys", "# future\nschema = 2\n[targets.claude.instances.second]\ndir = \"~/.claude_2\"\nhosts = [\"mac\"]\n", 2, 2},
		{"far future schema", "schema = 9223372036854775807\n", 9223372036854775807, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse("kitbag.toml", []byte(tc.body))
			var schema *manifest.SchemaError
			var positioned *manifest.Error
			if !errors.As(err, &schema) || !errors.As(err, &positioned) {
				t.Fatalf("Parse error %v is not a positioned *SchemaError", err)
			}
			if schema.Found != tc.found || schema.Supported != manifest.SchemaLatest || positioned.Key != "/schema" || positioned.Line != tc.line {
				t.Fatalf("schema %+v at %+v", schema, positioned)
			}
		})
	}
}

func TestParseKeepsOlderSchemaInvalid(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"schema = 0", "schema = -1"} {
		_, err := manifest.Parse("kitbag.toml", []byte(body))
		var schema *manifest.SchemaError
		if err == nil || errors.As(err, &schema) {
			t.Fatalf("%s: error %v, want a validation error without *SchemaError", body, err)
		}
	}
	if manifest.SchemaOldest != 1 || manifest.SchemaLatest != 1 {
		t.Fatalf("supported schemas %d..%d", manifest.SchemaOldest, manifest.SchemaLatest)
	}
}

type preflightCase struct {
	name, body, current, want string
	required                  string
	schema                    int64
	line                      int
}

func preflightCases() []preflightCase {
	return []preflightCase{
		{name: "minimum wins over unknown keys and schema", body: "schema = 2\nmin_tool_version = \"99.0.0\"\nfuture = { a = 1 }\n", current: "1.0.0", required: "99.0.0", line: 2},
		{name: "minimum with invalid values", body: "schema = 1\nmode = \"teleport\"\nmin_tool_version = \"2.0.0\"\n", current: "1.0.0", required: "2.0.0", line: 3},
		{name: "release outranks prerelease", body: "schema = 1\nmin_tool_version = \"1.0.0\"\n", current: "1.0.0-rc.1", required: "1.0.0", line: 2},
		{name: "schema after satisfied minimum", body: "min_tool_version = \"0.1.0\"\nschema = 3\n", current: "1.0.0", schema: 3, line: 2},
		{name: "satisfied minimum", body: "schema = 1\nmin_tool_version = \"1.0.0\"\n", current: "1.0.0"},
		{name: "unknown keys left to Parse", body: "schema = 1\nfuture = true\n", current: "1.0.0"},
		{name: "malformed minimum left to Parse", body: "schema = 1\nmin_tool_version = \"next\"\n", current: "1.0.0"},
		{name: "string schema left to Parse", body: "schema = \"2\"\n", current: "1.0.0"},
		{name: "older schema left to Parse", body: "schema = 0\n", current: "1.0.0"},
		{name: "syntax", body: "schema = [", current: "1.0.0", want: "toml:"},
		{name: "invalid running version", body: "schema = 1\n", current: "dev", want: "invalid tool version"},
	}
}

func TestPreflight(t *testing.T) {
	t.Parallel()
	for _, tc := range preflightCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := manifest.Preflight("kitbag.toml", []byte(tc.body), tc.current)
			switch {
			case tc.required != "":
				assertVersionError(t, err, tc)
			case tc.schema != 0:
				assertSchemaError(t, err, tc)
			case tc.want != "":
				assertOtherError(t, err, tc.want)
			case err != nil:
				t.Fatalf("Preflight = %v", err)
			}
		})
	}
}

func assertVersionError(t *testing.T, err error, tc preflightCase) {
	t.Helper()
	var version *manifest.VersionError
	var schema *manifest.SchemaError
	if !errors.As(err, &version) || version.Required != tc.required || version.Current != tc.current || errors.As(err, &schema) {
		t.Fatalf("error %v, want *VersionError requiring %s", err, tc.required)
	}
	assertLine(t, err, tc.line)
}

func assertSchemaError(t *testing.T, err error, tc preflightCase) {
	t.Helper()
	var version *manifest.VersionError
	var schema *manifest.SchemaError
	if !errors.As(err, &schema) || schema.Found != tc.schema || schema.Supported != manifest.SchemaLatest || errors.As(err, &version) {
		t.Fatalf("error %v, want *SchemaError for %d", err, tc.schema)
	}
	assertLine(t, err, tc.line)
}

func assertOtherError(t *testing.T, err error, want string) {
	t.Helper()
	var version *manifest.VersionError
	var schema *manifest.SchemaError
	if err == nil || errors.As(err, &version) || errors.As(err, &schema) || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v, want %q", err, want)
	}
}

func assertLine(t *testing.T, err error, line int) {
	t.Helper()
	var positioned *manifest.Error
	if !errors.As(err, &positioned) || positioned.Line != line {
		t.Fatalf("position %+v, want line %d", positioned, line)
	}
}
