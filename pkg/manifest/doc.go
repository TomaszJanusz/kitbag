// Package manifest parses and validates schema-v1 kitbag.toml intent.
// Parse checks the portable document contract; Load additionally checks repository
// sources through an injected filesystem. Neither function executes manifest
// content. Call CheckVersion before planning and ResolveHost to select overrides.
package manifest
