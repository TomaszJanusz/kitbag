// Package compatibility gates manifest consumers before host mutations.
package compatibility

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Event describes a blocked operation for notifications and future API consumers.
// Required is set when min_tool_version is unmet; Schema and SupportedSchema
// when the manifest schema is newer than this tool reads.
type Event struct {
	Type            string `json:"type"`
	Required        string `json:"required,omitempty"`
	Current         string `json:"current"`
	Repository      string `json:"repository"`
	Schema          int64  `json:"schema,omitempty"`
	SupportedSchema int64  `json:"supported_schema,omitempty"`
}

// running attaches the running version to a schema rejection, which manifest
// reports without it.
type running struct {
	err     error
	version string
}

func (e *running) Error() string { return e.err.Error() }
func (e *running) Unwrap() error { return e.err }

// FromError returns a typed tool-too-old event without persisting anything.
func FromError(err error, repository string) *Event {
	var version *manifest.VersionError
	if errors.As(err, &version) {
		return &Event{Type: "tool-too-old", Required: version.Required, Current: version.Current, Repository: repository}
	}
	var schema *manifest.SchemaError
	if !errors.As(err, &schema) {
		return nil
	}
	event := &Event{Type: "tool-too-old", Repository: repository, Schema: schema.Found, SupportedSchema: schema.Supported}
	var tool *running
	if errors.As(err, &tool) {
		event.Current = tool.version
	}
	return event
}

// Load reads the repository manifest and rejects one written for a newer tool
// (min_tool_version or schema) before full validation, then loads and checks it.
func Load(host platform.FS, repository, version string) (*manifest.Manifest, error) {
	file, data, err := read(host, repository)
	if err != nil {
		return nil, err
	}
	if err = preflight(file, data, version); err != nil {
		return nil, err
	}
	m, err := manifest.Load(host, file)
	if err != nil {
		return nil, err
	}
	return m, m.CheckVersion(version)
}

func read(host platform.FS, repository string) (string, []byte, error) {
	file := filepath.Join(repository, "kitbag.toml")
	if !filepath.IsAbs(file) {
		return "", nil, fmt.Errorf("manifest filename must be absolute and clean: %q", file)
	}
	data, err := host.ReadFile(file)
	if err != nil {
		return "", nil, fmt.Errorf("read manifest: %w", err)
	}
	return file, data, nil
}

func preflight(file string, data []byte, version string) error {
	err := manifest.Preflight(file, data, version)
	var schema *manifest.SchemaError
	if errors.As(err, &schema) {
		return &running{err: err, version: version}
	}
	return err
}

// Check rejects an incompatible local manifest before locks, recovery or Git writes.
// Compatibility is read before full validation, so an otherwise invalid manifest
// for a newer tool is still reported as such. Other load failures remain the
// command's responsibility; revert can work when the working copy is missing or
// damaged.
func Check(host platform.FS, repository, version string, emit func(Event)) error {
	file, data, err := read(host, repository)
	if err != nil {
		return nil
	}
	err = preflight(file, data, version)
	if event := FromError(err, repository); event != nil {
		if emit != nil {
			emit(*event)
		}
		return err
	}
	return nil
}
