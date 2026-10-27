package model

import (
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func previewSummary(p AgentPreview) []text.Message {
	var summary []text.Message
	if p.Manifest != "" {
		summary = append(summary, text.New(text.WizardManifestPath, text.Plain(p.Manifest)))
	}
	summary = append(summary, previewRoute(p))
	summary = append(summary, previewHosts(p.Change)...)
	if p.Directory != "" {
		summary = append(summary, text.New(text.WizardLocalDirectory, text.Plain(p.Directory)))
	}
	summary = append(summary, text.New(text.WizardPreviewSnapshot))
	if p.Dropped != "" {
		summary = append(summary, text.New(text.WizardRetainedDirectory, text.Plain(p.Dropped)))
	}
	if p.Effects == nil {
		return append(summary, text.New(text.WizardEffectsUnavailable))
	}
	if len(p.Effects) == 0 {
		summary = append(summary, text.New(text.WizardNoEffects))
	}
	for _, effect := range p.Effects {
		summary = append(summary, effectSentence(effect))
	}
	return summary
}

func effectSentence(e AgentEffect) text.Message {
	keys := map[string]text.Key{
		"backup": text.WizardEffectBackup, "write": text.WizardEffectCreate, "materialize": text.WizardEffectCreate,
		"merge": text.WizardEffectMerge, "remove": text.WizardEffectDelete, "discard": text.WizardEffectDelete,
		"restore": text.WizardEffectRestore, "unmerge": text.WizardEffectUnmerge, "release": text.WizardEffectRelease,
		"skip": text.WizardEffectSkip, "record": text.WizardEffectUnchanged, "unchanged": text.WizardEffectUnchanged,
	}
	key, known := keys[e.Kind]
	args := []text.Message{text.Plain(e.Path)}
	if !known {
		return text.New(text.WizardEffectUnknown, args[0], text.Plain(e.Kind))
	}
	if e.Kind == "release" && !e.Existing {
		key = text.WizardEffectForget
	}
	if key == text.WizardEffectCreate && e.Existing {
		key = text.WizardEffectReplace
	}
	switch e.Kind {
	case "backup":
		args = append(args, text.Plain(e.Backup))
	case "skip":
		args = append(args, text.Plain(e.Warning))
	}
	return text.New(key, args...)
}

func previewRoute(p AgentPreview) text.Message {
	switch {
	case p.Publication.Mode != "" && p.Publication.Mode != "direct":
		return text.New(text.WizardPublishManual)
	case p.Publication.Branch != "":
		return text.New(text.WizardPublishDirect, text.Plain(p.Publication.Branch))
	default:
		return text.New(text.WizardPublicationUnavailable)
	}
}

func previewHosts(change AgentChange) []text.Message {
	var summary []text.Message
	switch hosts := SplitHosts(change.Hosts); {
	case len(hosts) > 0:
		summary = append(summary, text.New(text.WizardPreviewHosts, text.Plain(strings.Join(hosts, ", "))))
	case change.AllHosts:
		summary = append(summary, text.New(text.WizardPreviewAllHosts))
	case change.Op == AgentAdd || change.Op == AgentEdit:
		summary = append(summary, text.New(text.WizardPreviewEveryHost))
	}
	return summary
}
