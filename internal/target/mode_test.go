package target_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestManifestModeSpecificity(t *testing.T) {
	t.Parallel()
	const primitive = "skills"
	const instanceName = "main"
	for _, tc := range []struct{ name, top, target, instance, primitive, instancePrimitive, want string }{
		{name: "default", want: ""},
		{name: "manifest", top: "copy", want: "copy"},
		{name: "target", top: "copy", target: "link", want: "link"},
		{name: "instance", target: "copy", instance: "link", want: "link"},
		{name: "primitive", instance: "copy", primitive: "link", want: "link"},
		{name: "instance primitive", primitive: "copy", instancePrimitive: "link", want: "link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &manifest.Manifest{Mode: tc.top, Targets: map[string]manifest.Target{"agent": {
				Mode: tc.target, Primitives: map[string]manifest.Primitive{primitive: {Mode: tc.primitive}},
				Instances: map[string]manifest.Instance{instanceName: {Mode: tc.instance, Primitives: map[string]manifest.Primitive{primitive: {Mode: tc.instancePrimitive}}}},
			}}}
			if got := target.ManifestMode(m, "agent", instanceName, primitive); got != tc.want {
				t.Fatalf("mode = %q; want %q", got, tc.want)
			}
		})
	}
}
