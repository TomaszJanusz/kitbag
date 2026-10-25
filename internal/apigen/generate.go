// Package apigen derives the reference and executable contract cases from the
// reviewed JSON Schema. It has no host operations or runtime API dependency.
package apigen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"strings"
)

const (
	testBoilerplateParts      = 2
	referenceBoilerplateParts = 3
)

type method struct {
	Name         string          `json:"name"`
	Summary      string          `json:"summary"`
	Request      string          `json:"request"`
	Result       string          `json:"result"`
	Valid        json.RawMessage `json:"valid"`
	Invalid      json.RawMessage `json:"invalid"`
	InvalidError int             `json:"invalid_error"`
}

type schema struct {
	Version string   `json:"x-api-version"`
	Methods []method `json:"x-methods"`
	Errors  []struct {
		ExitCode any `json:"exit_code"`
		Meaning  string
		Code     int
	} `json:"x-errors"`
	Notifications []struct{ Name, Summary string } `json:"x-notifications"`
}

// Generate returns deterministic Markdown and Go tests, both owned by schema.
func Generate(data []byte) (reference, tests []byte, err error) {
	var contract schema
	if err = json.Unmarshal(data, &contract); err != nil {
		return nil, nil, err
	}
	if contract.Version == "" || len(contract.Methods) == 0 {
		return nil, nil, errors.New("missing API contract")
	}
	reference = []byte(markdown(contract))
	parts := make([]string, 0, len(contract.Methods)+testBoilerplateParts)
	parts = append(parts, "// Code generated from docs/api/api-v1.schema.json; DO NOT EDIT.\n//go:build darwin || linux\n\npackage main\n\nimport \"testing\"\n\nfunc TestGeneratedAPIContracts(t *testing.T) {\n t.Parallel()\n")
	for _, m := range contract.Methods {
		parts = append(parts, fmt.Sprintf("t.Run(%q, func(t *testing.T){t.Parallel(); testAPIContract(t,%q,%q,%q,%d,%q,%q)})\n", m.Name, m.Name, compact(m.Valid), compact(m.Invalid), m.InvalidError, m.Request, m.Result))
	}
	parts = append(parts, "}\n")
	tests, err = format.Source([]byte(strings.Join(parts, "")))
	return reference, tests, err
}

func compact(data []byte) string {
	var out bytes.Buffer
	if json.Compact(&out, data) != nil {
		return "null"
	}
	return out.String()
}

func markdown(s schema) string {
	parts := make([]string, 0, len(s.Methods)+len(s.Errors)+len(s.Notifications)+referenceBoilerplateParts)
	parts = append(parts, "<!-- Generated from api-v1.schema.json; DO NOT EDIT. -->\n# Local API v"+s.Version+" reference\n\nSee [transport and compatibility](local-api.md). Every method uses the schema's\n`params` envelope and returns `{api_version, data}`. Unknown optional request\nfields are ignored. Method result types are in `$defs` of the schema.\n\n")
	for _, m := range s.Methods {
		parts = append(parts, "## `"+m.Name+"`\n\n"+m.Summary+"\n\nRequest: `$defs/"+m.Request+"`; result data: `$defs/"+m.Result+"`.\n\nExample method arguments (add `token` and `api_version`):\n\n```json\n"+compact(m.Valid)+"\n```\n\n")
	}
	parts = append(parts, "## Errors\n\n| JSON-RPC code | Meaning | CLI exit |\n| --- | --- | --- |\n")
	for _, e := range s.Errors {
		parts = append(parts, fmt.Sprintf("| %d | %s | %v |\n", e.Code, e.Meaning, e.ExitCode))
	}
	parts = append(parts, "\n## Notifications\n\n")
	for _, n := range s.Notifications {
		parts = append(parts, "- `"+n.Name+"`: "+n.Summary+"\n")
	}
	return strings.Join(parts, "")
}
