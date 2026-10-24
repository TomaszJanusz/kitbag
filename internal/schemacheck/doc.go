// Package schemacheck holds the quality-gate tests that validate the JSON
// Schema files under schemas/ against the TOML examples next to them. It is
// not production code: pkg/manifest (KIT-12) will own the real parser, and the
// schema libraries used here are test-only dependencies.
package schemacheck
