// Package manifest parses and validates schema-1 and schema-2 kitbag.toml intent.
// Parse checks the portable document contract; Load additionally checks repository
// sources through an injected filesystem. Neither function executes manifest
// content. Preflight reports a manifest written for a newer tool before full
// validation. Call CheckVersion before planning and ResolveHost to select overrides.
package manifest
