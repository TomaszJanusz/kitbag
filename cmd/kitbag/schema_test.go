package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/rogpeppe/go-internal/testscript"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func planSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return namedSchema(t, "plan")
}

func namedSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../schemas/" + name + ".schema.json") //nolint:gosec // G304: name is a fixed test-owned schema name, never user input.
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	url := "https://kitbag.sh/schemas/" + name + ".schema.json"
	if err = compiler.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	if name == "adopt" {
		// Remaining operations reuse the plan schema's operation definition.
		addSchemaResource(t, compiler, "plan")
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func addSchemaResource(t *testing.T, compiler *jsonschema.Compiler, name string) {
	t.Helper()
	data, err := os.ReadFile("../../schemas/" + name + ".schema.json") //nolint:gosec // G304: name is a fixed test-owned schema name, never user input.
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err = compiler.AddResource("https://kitbag.sh/schemas/"+name+".schema.json", doc); err != nil {
		t.Fatal(err)
	}
}

func validatePlanCommand(schema *jsonschema.Schema) func(*testscript.TestScript, bool, []string) {
	return func(ts *testscript.TestScript, neg bool, args []string) {
		if neg || len(args) != 1 {
			ts.Fatalf("usage: planschema file")
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(ts.ReadFile(args[0])))
		ts.Check(err)
		ts.Check(schema.Validate(doc))
	}
}

func TestPlanSchemaRejectsDrift(t *testing.T) {
	t.Parallel()
	schema := planSchema(t)
	for _, data := range []string{`{"plan_schema":2,"operations":[],"observations":[]}`, `{"plan_schema":1,"operations":null,"observations":[]}`, `{"plan_schema":1,"operations":[],"observations":[],"content":"secret"}`} {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(data))
		if err != nil {
			t.Fatal(err)
		}
		if err = schema.Validate(doc); err == nil {
			t.Fatalf("accepted invalid plan: %s", data)
		}
	}
}

func TestLifecycleSchemasRejectDrift(t *testing.T) {
	t.Parallel()
	for name, documents := range map[string][]string{
		"adopt": {
			`{"adopt_schema":2,"dry_run":false,"adopted":[],"moved":[],"remaining":[],"undeclared":[]}`,
			`{"adopt_schema":1,"dry_run":false,"adopted":null,"moved":[],"remaining":[],"undeclared":[]}`,
			`{"adopt_schema":1,"dry_run":false,"adopted":[{"path":"/a","kind":"other","legacy":false}],"moved":[],"remaining":[],"undeclared":[]}`,
			`{"adopt_schema":1,"dry_run":false,"adopted":[],"moved":[],"remaining":[{"kind":"unknown","path":"/a","classification":"in sync"}],"undeclared":[]}`,
		},
		"history": {
			`{"history_schema":2,"plans":[]}`,
			`{"history_schema":1,"plans":[{"number":0,"id":"x","applied_at":"2026-01-01T00:00:00Z","adopted":false,"paths":[]}]}`,
		},
	} {
		schema := namedSchema(t, name)
		for _, data := range documents {
			doc, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(data))
			if err != nil {
				t.Fatal(err)
			}
			if err = schema.Validate(doc); err == nil {
				t.Fatalf("accepted invalid %s document: %s", name, data)
			}
		}
	}
}

func TestDoctorSchemaRejectsDrift(t *testing.T) {
	t.Parallel()
	schema := namedSchema(t, "doctor")
	for _, data := range []string{
		`{"doctor_schema":2,"checks":[]}`,
		`{"doctor_schema":1,"checks":null}`,
		`{"doctor_schema":1,"checks":[],"contents":"secret"}`,
		`{"doctor_schema":1,"checks":[{"id":"remote","status":"FAIL","message":"broken","fix":""}]}`,
		`{"doctor_schema":1,"checks":[{"id":"remote","status":"INFO","message":"broken","fix":"fix"}]}`,
	} {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(data))
		if err != nil {
			t.Fatal(err)
		}
		if err = schema.Validate(doc); err == nil {
			t.Fatalf("accepted invalid doctor report: %s", data)
		}
	}
}

func validateConfigCommand(schema *jsonschema.Schema) func(*testscript.TestScript, bool, []string) {
	return func(ts *testscript.TestScript, neg bool, args []string) {
		if neg || len(args) != 1 {
			ts.Fatalf("usage: configschema file")
		}
		var tree map[string]any
		_, err := toml.Decode(ts.ReadFile(args[0]), &tree)
		ts.Check(err)
		data, err := json.Marshal(tree)
		ts.Check(err)
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		ts.Check(err)
		ts.Check(schema.Validate(value))
	}
}
