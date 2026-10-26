package manifest

import (
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// Manifest schema versions this package reads.
const (
	SchemaOldest = 1
	SchemaLatest = 1
)

// SchemaError means the manifest declares a schema newer than SchemaLatest, so
// the running tool is too old to read it. The CLI classifies it like
// VersionError (exit code 4).
type SchemaError struct {
	Found     int64
	Supported int64
}

// Error returns the declared and newest supported schema versions.
func (e *SchemaError) Error() string {
	return fmt.Sprintf("manifest schema %d is newer than this kitbag reads (schema %d); update kitbag", e.Found, e.Supported)
}

// Preflight reads only the compatibility keys of a TOML document, before and
// independently of full validation, so that a manifest written for a newer
// tool is reported as such rather than as unknown keys or values. It returns a
// positioned error wrapping *VersionError when min_tool_version is above
// current, else one wrapping *SchemaError when schema is above SchemaLatest.
// Undecodable TOML returns the positioned decode error; current must be a valid
// semantic version. Other problems, including a malformed min_tool_version or
// schema, are left to Parse.
func Preflight(filename string, data []byte, current string) error {
	if !matches(textRule(versionPattern), current) {
		return fmt.Errorf("invalid tool version %q", current)
	}
	d := &document{file: filename, positions: map[string]position{}}
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return d.decodeError(err)
	}
	d.index(data)
	if required, ok := tree["min_tool_version"].(string); ok && matches(textRule(versionPattern), required) && compareVersion(current, required) < 0 {
		v := &VersionError{Required: required, Current: current}
		return d.fail("/min_tool_version", v.Error(), v)
	}
	return d.preflightSchema(tree)
}

func (d *document) preflightSchema(tree map[string]any) error {
	if found, ok := tree["schema"].(int64); ok && found > SchemaLatest {
		s := &SchemaError{Found: found, Supported: SchemaLatest}
		return d.fail("/schema", s.Error(), s)
	}
	return nil
}
