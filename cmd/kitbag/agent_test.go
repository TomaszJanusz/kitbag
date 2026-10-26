package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestAgentOutputFailures(t *testing.T) {
	t.Parallel()
	result := discover.Snapshot{Kinds: []string{"claude"}, Instances: []discover.Instance{{ListedCandidate: discover.ListedCandidate{Kind: "claude"}}}, Candidates: []discover.ListedCandidate{{Kind: "claude"}}, Warnings: []string{"warning"}}
	c := &commandLine{stdout: failedWriter{}, stderr: failedWriter{}}
	for _, asJSON := range []bool{false, true} {
		if err := c.printAgents(result, asJSON); err == nil {
			t.Fatal("ignored output failure")
		}
	}
	if err := c.printAgentKind("claude", result); err == nil {
		t.Fatal("ignored instance output failure")
	}
	result.Instances = nil
	if err := c.printAgentKind("claude", result); err == nil {
		t.Fatal("ignored candidate output failure")
	}
	c.stdout = &bytes.Buffer{}
	if err := c.printAgents(result, false); err == nil {
		t.Fatal("ignored warning output failure")
	}
}

func TestAgentsSchemaRejectsDrift(t *testing.T) {
	t.Parallel()
	result, err := agentHomes().List(noGit(), nil, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	schema := namedSchema(t, "agents")
	var doc map[string]any
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(doc); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]any{"agents_schema": 2, "candidates": nil, "instances": nil, "contents": "secret", "kinds": []any{"unknown"}} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			var bad map[string]any
			if decodeErr := json.Unmarshal(data, &bad); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			bad[key] = value
			if validateErr := schema.Validate(bad); validateErr == nil {
				t.Fatalf("accepted invalid %s", key)
			}
		})
	}
}

type probeFailure struct{ platform.Host }

func (h probeFailure) ReadDir(string) ([]fs.DirEntry, error) { return nil, fs.ErrPermission }

func TestAgentProbeFailureCode(t *testing.T) {
	t.Parallel()
	var out, diagnostic bytes.Buffer
	code := run("linux", probeFailure{noGit()}, []string{"agent", "list", "--probe"}, &out, &diagnostic)
	if code != 1 || out.Len() != 0 || !strings.Contains(diagnostic.String(), "probe HOME") {
		t.Fatalf("code=%d out=%s err=%s", code, &out, &diagnostic)
	}
}

func TestAgentDefaultConfigIsOptionalWithoutGit(t *testing.T) {
	t.Parallel()
	var out, diagnostic bytes.Buffer
	code := run("linux", noGit(), []string{"agent", "list", "--json"}, &out, &diagnostic)
	if code != 0 || diagnostic.Len() != 0 || !strings.Contains(out.String(), "No repository configured") {
		t.Fatalf("code=%d out=%s err=%s", code, &out, &diagnostic)
	}
}
