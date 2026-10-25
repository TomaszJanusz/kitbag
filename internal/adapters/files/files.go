// Package files declares the manifest's [[files]] entries: arbitrary
// repository files or directories linked or copied to home destinations. It is
// not an agent adapter, so it imports no agent package. See docs/generic.md.
package files

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Adapter implements target.Adapter for [[files]].
type Adapter struct {
	home string
}

var _ target.Adapter = Adapter{}

// New returns an adapter that expands "~/" destinations below home, the
// applying user's absolute, clean home directory resolved by the caller.
func New(home string) Adapter {
	return Adapter{home: home}
}

// Declare returns one materialized target per host-resolved files entry. Each
// may back up a pre-existing unmanaged regular file once; the planner decides.
// A missing source is absent desired state, as for every materialized target.
func (a Adapter) Declare(_ manifest.FileSystem, repository string, intent *manifest.Manifest) ([]target.Target, error) {
	if !filepath.IsAbs(a.home) || filepath.Clean(a.home) != a.home {
		return nil, fmt.Errorf("files: home %q is not absolute and clean", a.home)
	}
	declared := make([]target.Target, 0, len(intent.Files))
	for _, file := range intent.Files {
		mode := file.Mode
		if mode == "" {
			mode = intent.Mode
		}
		declared = append(declared, target.Target{
			Path:       filepath.Join(a.home, filepath.FromSlash(strings.TrimPrefix(file.Dest, "~/"))),
			SourcePath: filepath.Join(repository, filepath.FromSlash(file.Src)),
			Kind:       target.Materialized,
			Backup:     target.Once,
			Mode:       mode,
			Owners:     []state.Owner{{Primitive: "files", AgentInstance: "files"}},
		})
	}
	return declared, nil
}
