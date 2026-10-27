// Code generated from docs/api/api-v1.schema.json; DO NOT EDIT.
//go:build darwin || linux

package main

import "testing"

func TestGeneratedAPIContracts(t *testing.T) {
	t.Parallel()
	t.Run("status.get", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "status.get", "{}", "{\"api_version\":\"2.0\"}", -32001, "status_getRequest", "status")
	})
	t.Run("plan.get", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "plan.get", "{}", "{\"token\":\"\"}", -32000, "plan_getRequest", "plan")
	})
	t.Run("sync.run", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "sync.run", "{\"no_pull\":true}", "{\"force\":\"wrong\"}", -32602, "sync_runRequest", "sync")
	})
	t.Run("drift.list", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "drift.list", "{}", "{\"options\":5}", -32602, "drift_listRequest", "drift")
	})
	t.Run("config.get", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "config.get", "{}", "{\"options\":5}", -32602, "config_getRequest", "config")
	})
	t.Run("config.set", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "config.set", "{\"config\":\"schema = 1\\n[repo]\\nurl = \\\"https://example.invalid/config.git\\\"\\npath = \\\"~/repo\\\"\\n\"}", "{\"config\":5}", -32602, "config_setRequest", "config")
	})
	t.Run("agents.list", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "agents.list", "{\"probe\":false,\"env\":{\"CLAUDE_CONFIG_DIR\":\"\"}}", "{\"probe\":\"yes\"}", -32602, "agents_listRequest", "agents")
	})
	t.Run("agents.add", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "agents.add", "{\"agent\":{\"kind\":\"claude\",\"name\":\"third\",\"dir\":\"~/.claude_3\"},\"dry_run\":true}", "{\"agent\":5}", -32602, "agents_addRequest", "agentChangeResult")
	})
	t.Run("agents.edit", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "agents.edit", "{\"agent\":{\"target\":\"claude\",\"name\":\"second\",\"dir\":\"~/.claude-work\"},\"dry_run\":true}", "{\"agent\":{\"name\":5}}", -32602, "agents_editRequest", "agentChangeResult")
	})
	t.Run("agents.remove", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "agents.remove", "{\"agent\":{\"target\":\"claude\",\"name\":\"second\"},\"dry_run\":true}", "{\"dry_run\":\"yes\"}", -32602, "agents_removeRequest", "agentChangeResult")
	})
	t.Run("manifest.upgrade", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "manifest.upgrade", "{\"dry_run\":true}", "{\"dry_run\":\"yes\"}", -32602, "manifest_upgradeRequest", "manifestUpgradeResult")
	})
	t.Run("events.subscribe", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "events.subscribe", "{}", "{\"options\":5}", -32602, "events_subscribeRequest", "subscription")
	})
	t.Run("version.get", func(t *testing.T) {
		t.Parallel()
		testAPIContract(t, "version.get", "{}", "{\"options\":5}", -32602, "version_getRequest", "version")
	})
}
