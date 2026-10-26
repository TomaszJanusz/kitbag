package manifest

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Parse decodes a complete schema-v1 document and validates types, keys, portable
// paths, names and cross-references. It never accesses the host. A schema newer
// than SchemaLatest is rejected with a positioned error wrapping *SchemaError
// before any other validation; call Preflight first to also check
// min_tool_version that early. Use Load to also validate source existence,
// confinement and skill-directory contents.
func Parse(filename string, data []byte) (*Manifest, error) {
	d := &document{file: filename, positions: map[string]position{}}
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return nil, d.decodeError(err)
	}
	d.index(data)
	if err := d.preflightSchema(tree); err != nil {
		return nil, err
	}
	if err := d.validate(manifestRule(), tree, ""); err != nil {
		return nil, err
	}
	if err := d.pathCollisions(); err != nil {
		return nil, err
	}
	m := &Manifest{doc: d}
	if err := toml.Unmarshal(data, m); err != nil {
		return nil, d.decodeError(err)
	}
	if err := m.validateRelations(); err != nil {
		return nil, err
	}
	return m, nil
}

// Load reads a manifest at an absolute, clean filename and validates all local
// repository sources relative to its directory through fsys. Import Path names
// refer to foreign repositories and are not statted locally. Host instruction
// templates may be absent, as allowed by the contract.
func Load(fsys FileSystem, filename string) (*Manifest, error) {
	if !filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
		return nil, fmt.Errorf("manifest filename must be absolute and clean: %q", filename)
	}
	data, err := fsys.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := Parse(filename, data)
	if err != nil {
		return nil, err
	}
	if err = m.validateSources(fsys, filepath.Dir(filename)); err != nil {
		return nil, err
	}
	return m, nil
}

func (d *document) decodeError(err error) error {
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		line, column := decode.Position()
		return &Error{File: d.file, Line: line, Column: column, Message: err.Error(), Cause: err}
	}
	return d.fail("", err.Error(), err)
}
