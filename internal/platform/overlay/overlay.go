//go:build !windows

// Package overlay replaces selected environment variables of a host, so a
// long-running daemon can answer a request with the caller's relocation
// variables (ADR 12, section 1). Files, processes and every other variable
// remain the host's own.
package overlay

import (
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/xdg"
)

// Environment returns host with keys resolved from values; a key absent from
// values is unset. Agent homes and kitbag's directories are resolved again
// from the overlaid variables. Without keys it returns host unchanged.
func Environment(host platform.Host, keys []string, values map[string]string) platform.Host {
	if len(keys) == 0 {
		return host
	}
	return &environment{Host: host, keys: slices.Clone(keys), values: values}
}

type environment struct {
	platform.Host
	values map[string]string
	keys   []string
}

// LookupEnv answers overlaid keys from the request and others from the host.
func (e *environment) LookupEnv(key string) (string, bool) {
	if slices.Contains(e.keys, key) {
		value, ok := e.values[key]
		return value, ok
	}
	return e.Host.LookupEnv(key)
}

// AgentHome resolves an agent home with the overlaid variables.
func (e *environment) AgentHome(override, rel string) (string, error) {
	return xdg.AgentHome(e.LookupEnv, override, rel)
}

// Dirs resolves kitbag's directories with the overlaid variables.
func (e *environment) Dirs() (platform.Dirs, error) { return xdg.Dirs(e.LookupEnv) }
