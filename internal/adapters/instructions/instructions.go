// Package instructions renders the manifest's global instructions for every
// adapter. An adapter chooses a Strategy and a destination; only this package
// formats instruction files, so agents cannot drift apart in how they render.
package instructions

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Strategy selects how an agent receives its instructions.
type Strategy string

// Supported strategies. Import and Concat generate a managed file starting with
// Marker; Link materializes the effective source itself.
const (
	// Import writes one native "@<absolute path>" import line per source (Claude Code).
	Import Strategy = "import"
	// Concat writes the source and host addition contents (Codex).
	Concat Strategy = "concat"
	// Link materializes the single effective source for agents that follow links.
	Link Strategy = "link"
)

// Request describes one instructions destination of one agent instance.
// Instructions must come from a host-resolved manifest (Manifest.ResolveHost).
type Request struct {
	Instructions *manifest.Instructions
	// Repository is the absolute, clean working copy root.
	Repository string
	// Path is the absolute, clean destination chosen by the adapter.
	Path string
	// Agent is the agent kind used to look up instructions.per_agent.
	Agent    string
	Strategy Strategy
	Owners   []state.Owner
}

// Result is the declaration for Request.Path plus non-blocking warnings, such
// as a missing host addition. Target.Warning instead marks the target
// unsupported, which makes the planner skip it and keep previous ownership.
type Result struct {
	Warnings []string
	Target   target.Target
}

// Choose returns the requested render style (primitives.instructions.render),
// or fallback when unset, and rejects styles the adapter does not support.
func Choose(render string, fallback Strategy, supported ...Strategy) (Strategy, error) {
	chosen := fallback
	if render != "" {
		chosen = Strategy(render)
	}
	if !slices.Contains(supported, chosen) {
		return "", fmt.Errorf("instructions render %q is not supported by this agent", chosen)
	}
	return chosen, nil
}

// Declare renders the instructions for req. It only reads through fsys, never
// the destination, and is deterministic: equal inputs give equal declarations.
// Every declaration may back up an unmanaged destination once.
func Declare(fsys manifest.FileSystem, req Request) (Result, error) {
	if err := req.validate(); err != nil {
		return Result{}, err
	}
	in, err := gather(fsys, req)
	if err != nil {
		return Result{}, err
	}
	result := Result{Warnings: in.warnings, Target: target.Target{
		Path:   req.Path,
		Kind:   target.Generated,
		Backup: target.Once,
		Owners: slices.Clone(req.Owners),
	}}
	if req.Strategy == Link {
		result.Target.Kind = target.Materialized
	}
	if in.unsupported != "" {
		result.Target.Warning = in.unsupported
		return result, nil
	}
	switch req.Strategy {
	case Import:
		result.Target.Content = renderImport(in.paths)
	case Concat:
		parts, err := readAll(fsys, in.paths)
		if err != nil {
			return Result{}, err
		}
		result.Target.Content = renderConcat(parts)
	case Link:
		if len(in.paths) > 1 {
			result.Target.Warning = "instructions: link render cannot include host addition " + req.Instructions.Hosts + "; use import or concat"
			return result, nil
		}
		result.Target.SourcePath = in.paths[0]
	}
	return result, nil
}

func (req Request) validate() error {
	switch {
	case req.Instructions == nil || req.Instructions.Source == "":
		return errors.New("instructions: no source declared")
	case strings.Contains(req.Instructions.Hosts, "{host}"):
		return errors.New("instructions: hosts template is not host-resolved")
	case req.Agent == "":
		return errors.New("instructions: agent kind is required")
	case !absolute(req.Repository) || !absolute(req.Path):
		return errors.New("instructions: repository and destination must be absolute and clean")
	case req.Strategy != Import && req.Strategy != Concat && req.Strategy != Link:
		return fmt.Errorf("instructions: unknown strategy %q", req.Strategy)
	}
	return nil
}

func absolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

// inputs holds the absolute effective source followed by the present host addition.
type inputs struct {
	unsupported string
	paths       []string
	warnings    []string
}

func gather(fsys manifest.FileSystem, req Request) (inputs, error) {
	var in inputs
	source := req.Instructions.Source
	if override := req.Instructions.PerAgent[req.Agent]; override != "" {
		source = override
	}
	kind, err := lstat(fsys, join(req.Repository, source))
	if err != nil {
		return in, err
	}
	if kind != regularFile {
		in.unsupported = "instructions: source " + source + " is " + string(kind)
		return in, nil
	}
	in.paths = append(in.paths, join(req.Repository, source))
	if req.Instructions.Hosts == "" {
		return in, nil
	}
	host := join(req.Repository, req.Instructions.Hosts)
	if kind, err = lstat(fsys, host); err != nil {
		return in, err
	}
	switch kind {
	case regularFile:
		in.paths = append(in.paths, host)
	case missingFile:
		in.warnings = append(in.warnings, "instructions: host addition "+req.Instructions.Hosts+" not found; using "+source+" alone")
	case otherFile:
		in.unsupported = "instructions: host addition " + req.Instructions.Hosts + " is " + string(kind)
	}
	return in, nil
}

// fileKind classifies an input without following links, as the planner does.
type fileKind string

const (
	regularFile fileKind = "a regular file"
	missingFile fileKind = "missing"
	otherFile   fileKind = "not a regular file"
)

func lstat(fsys manifest.FileSystem, path string) (fileKind, error) {
	info, err := fsys.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return missingFile, nil
	case err != nil:
		return "", fmt.Errorf("instructions: %w", err)
	case !info.Mode().IsRegular():
		return otherFile, nil
	}
	return regularFile, nil
}

func join(repository, relative string) string {
	return filepath.Join(repository, filepath.FromSlash(relative))
}

func readAll(fsys manifest.FileSystem, paths []string) ([][]byte, error) {
	parts := make([][]byte, 0, len(paths))
	for _, path := range paths {
		data, err := fsys.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("instructions: %w", err)
		}
		parts = append(parts, data)
	}
	return parts, nil
}
