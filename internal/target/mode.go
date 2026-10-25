package target

import "github.com/TomaszJanusz/kitbag/pkg/manifest"

// ManifestMode resolves a primitive's most specific manifest policy after
// ResolveHost has folded host/host-target overrides. Adapters place this value
// in Target.Mode; apply separately combines it with machine policy (copy wins).
// Listed files use their own Mode, falling back to intent.Mode.
func ManifestMode(intent *manifest.Manifest, targetName, instanceName, primitive string) string {
	t := intent.Targets[targetName]
	instance := t.Instances[instanceName]
	mode := intent.Mode
	for _, candidate := range []string{t.Mode, instance.Mode, t.Primitives[primitive].Mode, instance.Primitives[primitive].Mode} {
		if candidate != "" {
			mode = candidate
		}
	}
	return mode
}
