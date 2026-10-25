package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func planSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../schemas/plan.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const url = "https://kitbag.sh/schemas/plan.schema.json"
	if err = compiler.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	return schema
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
