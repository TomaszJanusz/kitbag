// Package compatibility gates manifest consumers before host mutations.
package compatibility

import (
	"errors"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Event describes a blocked operation for notifications and future API consumers.
type Event struct {
	Type       string `json:"type"`
	Required   string `json:"required"`
	Current    string `json:"current"`
	Repository string `json:"repository"`
}

// FromError returns a typed tool-too-old event without persisting anything.
func FromError(err error, repository string) *Event {
	var version *manifest.VersionError
	if !errors.As(err, &version) {
		return nil
	}
	return &Event{Type: "tool-too-old", Required: version.Required, Current: version.Current, Repository: repository}
}

// Check rejects an incompatible local manifest before locks, recovery or Git writes.
// Other load failures remain the command's responsibility; revert can work when
// the working copy is missing or damaged.
func Check(host platform.FS, repository, version string, emit func(Event)) error {
	m, err := manifest.Load(host, filepath.Join(repository, "kitbag.toml"))
	if err != nil {
		return nil
	}
	err = m.CheckVersion(version)
	if event := FromError(err, repository); event != nil {
		if emit != nil {
			emit(*event)
		}
		return err
	}
	return nil
}
