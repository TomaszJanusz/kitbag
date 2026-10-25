package plan

import (
	"bytes"
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func assemble(fsys platform.FS, req Request) ([]target.Target, error) {
	byPath := map[string]target.Target{}
	for _, adapter := range req.Adapters {
		declarations, err := adapter.Declare(fsys, req.Repository, req.Manifest)
		if err != nil {
			return nil, fmt.Errorf("declare targets: %w", err)
		}
		for _, declaration := range declarations {
			if err = addDeclaration(byPath, declaration, req.Repository); err != nil {
				return nil, err
			}
		}
	}
	result := make([]target.Target, 0, len(byPath))
	for _, declaration := range byPath {
		result = append(result, declaration)
	}
	slices.SortFunc(result, func(a, b target.Target) int { return strings.Compare(a.Path, b.Path) })
	for i, declaration := range result {
		for _, previous := range result[:i] {
			if foldedOverlap(previous.Path, declaration.Path) {
				return nil, invalid("overlapping destinations: " + previous.Path + " and " + declaration.Path)
			}
		}
	}
	return result, nil
}

func addDeclaration(byPath map[string]target.Target, d target.Target, repository string) error {
	if err := validateDeclaration(d, repository); err != nil {
		return err
	}
	d.Content = bytes.Clone(d.Content)
	d.Owners = sortedOwners(d.Owners)
	if previous, ok := byPath[d.Path]; ok {
		if !sameDeclaration(previous, d) {
			return invalid("conflicting declarations: " + d.Path)
		}
		d.Owners = sortedOwners(append(slices.Clone(previous.Owners), d.Owners...))
	}
	byPath[d.Path] = d
	return nil
}

func validateDeclaration(d target.Target, repository string) error {
	if err := validatePolicy(d); err != nil {
		return err
	}
	if !validPath(d.Path) || foldedOverlap(repository, d.Path) {
		return invalid("destination overlaps repository or is not absolute and clean: " + d.Path)
	}
	if len(d.Owners) == 0 {
		return invalid("missing owners: " + d.Path)
	}
	for _, owner := range d.Owners {
		if owner.Primitive == "" || owner.AgentInstance == "" {
			return invalid("empty owner: " + d.Path)
		}
	}
	if d.Backup != target.Refuse && d.Backup != target.Once {
		return invalid("unknown backup policy")
	}
	return validateSource(d, repository)
}

func validateSource(d target.Target, repository string) error {
	if d.SourcePath != "" && (!validPath(d.SourcePath) || !within(repository, d.SourcePath)) {
		return invalid("source outside repository: " + d.SourcePath)
	}
	return validateOutput(d)
}

func validateOutput(d target.Target) error {
	if d.Warning != "" {
		return nil
	}
	switch d.Kind {
	case target.Materialized:
		if !materializedValid(d) {
			return invalid("materialized target requires only source")
		}
	case target.Generated:
		if !generatedValid(d) {
			return invalid("generated target requires content only")
		}
	case target.Merged:
		if !mergedValid(d) {
			return invalid("merged target requires merger and one fragment")
		}
	default:
		return invalid("unknown target kind")
	}
	return nil
}

func sortedOwners(owners []state.Owner) []state.Owner {
	result := slices.Clone(owners)
	slices.SortFunc(result, func(a, b state.Owner) int {
		if n := cmp.Compare(a.AgentInstance, b.AgentInstance); n != 0 {
			return n
		}
		return cmp.Compare(a.Primitive, b.Primitive)
	})
	return slices.Compact(result)
}

func sameDeclaration(a, b target.Target) bool {
	// Independently composed mergers cannot safely share a destination yet.
	return a.Kind != target.Merged && a.Kind == b.Kind && a.Mode == b.Mode && a.InstanceDir == b.InstanceDir && a.SourcePath == b.SourcePath && bytes.Equal(a.Content, b.Content) && a.Backup == b.Backup && a.Warning == b.Warning
}

func foldedOverlap(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	return foldPrefix(a, b) || foldPrefix(b, a)
}

func foldPrefix(parent, child string) bool {
	for child != "." && child != "" {
		next := filepath.Dir(child)
		if next == child {
			return false
		}
		if strings.EqualFold(parent, next) {
			return true
		}
		child = next
	}
	return false
}

func materializedValid(d target.Target) bool {
	return d.SourcePath != "" && d.Content == nil && d.Merger == nil
}
func generatedValid(d target.Target) bool { return d.SourcePath == "" && d.Merger == nil }
func mergedValid(d target.Target) bool {
	return d.Merger != nil && (d.SourcePath == "" || d.Content == nil)
}

func validatePolicy(d target.Target) error {
	if d.Mode != "" && d.Mode != "link" && d.Mode != "copy" {
		return invalid("unknown materialization mode")
	}
	if d.InstanceDir != "" && !validPath(d.InstanceDir) {
		return invalid("invalid instance directory")
	}
	return nil
}
