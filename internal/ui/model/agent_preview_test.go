package model

import (
	"strings"
	"testing"
)

func TestAgentPreviewPlainConsequences(t *testing.T) {
	t.Parallel()
	p := AgentPreview{Manifest: "/clone/kitbag.toml", Directory: "/home/u/.claude-work", Dropped: "~/.old", Publication: AgentPublication{Mode: "direct", Branch: "config"}, Effects: []AgentEffect{
		{Path: "/home/u/.claude-work/rules", Kind: "backup", Backup: "/backup/rules", Existing: true},
		{Path: "/home/u/.claude-work/rules", Kind: "materialize", Existing: true},
		{Path: "/home/u/.old/owned", Kind: "discard", Existing: true},
		{Path: "/home/u/.old/changed", Kind: "release", Existing: true},
		{Path: "/home/u/.old/settings", Kind: "restore", Existing: true},
	}}
	summary := previewSummary(p)
	checkText(t, summary...)
	rendered := make([]string, 0, len(summary))
	for _, sentence := range summary {
		rendered = append(rendered, render(sentence))
	}
	body := strings.Join(rendered, "\n")
	for _, sentence := range []string{"/clone/kitbag.toml", "push this change to branch config", "Every host reads it on its next sync", "Local configuration directory: /home/u/.claude-work.", "Back up existing", "Replace existing", "Delete the owned output", "Leave /home/u/.old/changed in place", "Restore the original entry", "files KitBag does not own"} {
		expect(t, strings.Contains(body, sentence), "missing %q in %s", sentence, body)
	}
	p.Publication.Mode = "pr"
	summary = previewSummary(p)
	checkText(t, summary...)
	expect(t, strings.Contains(render(summary[1]), "Apply writes no manifest"), "manual route %s", render(summary[1]))
}

// TestAgentPreviewNamesMachines says where a restricted instance is used.
func TestAgentPreviewNamesMachines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		want   string
		change AgentChange
	}{
		{"Used only on mac, ci; other machines ignore this instance.", AgentChange{Hosts: "mac,ci"}},
		{"Used on every machine again.", AgentChange{AllHosts: true}},
		{"Used on every machine; machines without its directory will warn and skip it.", AgentChange{Op: AgentAdd}},
		{"Used on every machine; machines without its directory will warn and skip it.", AgentChange{Op: AgentEdit}},
		{"", AgentChange{Op: AgentRemove}},
	} {
		summary := previewSummary(AgentPreview{Change: tc.change, Publication: AgentPublication{Mode: "direct", Branch: "main"}, Effects: []AgentEffect{}})
		checkText(t, summary...)
		got := ""
		for _, sentence := range summary {
			if line := render(sentence); strings.HasPrefix(line, "Used ") {
				got = line
			}
		}
		expect(t, got == tc.want, "%+v: %q", tc.change, got)
	}
}

func TestAgentEffectSentencesAndOlderDaemon(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"backup", "write", "materialize", "merge", "remove", "discard", "restore", "unmerge", "release", "skip", "record", "unchanged", "future"} {
		checkText(t, effectSentence(AgentEffect{Path: "/file", Kind: kind, Backup: "/backup", Warning: "unsafe"}))
	}
	summary := previewSummary(AgentPreview{})
	expect(t, strings.Contains(render(summary[len(summary)-1]), "effects are unavailable"), "old daemon invented effects")
	summary = previewSummary(AgentPreview{Effects: []AgentEffect{}})
	expect(t, strings.Contains(render(summary[len(summary)-1]), "No local file changes"), "empty plan summary")
}
