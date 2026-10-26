package workflow_test

import (
	"slices"
	"testing"
)

func TestAgentsListsDeclaredInstances(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, "schema=1\n[targets.claude]\nagent='claude'\n[targets.claude.instances.second]\ndir='~/.claude_2'\n")
	snapshot, err := f.s.Agents(false)
	expect(t, err == nil && snapshot.Host == "mac" && snapshot.Schema == 1, "snapshot", snapshot)
	expect(t, len(snapshot.Instances) == 1 && snapshot.Instances[0].Name == "second" && snapshot.Instances[0].ActiveHere, "instances", snapshot.Instances)
	expect(t, slices.Equal(f.s.AgentVariables(), []string{"CLAUDE_CONFIG_DIR"}), "variables", f.s.AgentVariables())
}

func TestAgentsWithoutManifestStillDiscovers(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, "schema=1\n")
	if err := f.host.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.s.Agents(false)
	expect(t, err == nil && snapshot.Schema == 0 && len(snapshot.Candidates) == 1, "snapshot", snapshot)
	expect(t, slices.Contains(snapshot.Warnings, "No manifest yet; listing detected agent directories only."), "warnings", snapshot.Warnings)
}

func TestAgentsReportsAnInvalidManifest(t *testing.T) {
	t.Parallel()
	f := agentSetup(t, "schema=1\n[targets.claude]\nagnet='claude'\n")
	_, err := f.s.Agents(false)
	expect(t, err != nil, "invalid manifest", err)
}
