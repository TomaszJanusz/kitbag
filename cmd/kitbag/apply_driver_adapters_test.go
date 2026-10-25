//go:build darwin || linux

package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/adapters/files"
	"github.com/TomaszJanusz/kitbag/internal/adapters/generic"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// selection is the test driver's adapter set and whether Git includes run.
type selection struct {
	adapters []target.Adapter
	git      bool
}

// driverAdapters selects real adapters by name; an empty list keeps the fixture.
func driverAdapters(names, home string, fixture target.Adapter) (selection, error) {
	if names == "" {
		return selection{adapters: []target.Adapter{fixture}}, nil
	}
	var s selection
	for _, name := range strings.Split(names, ",") {
		switch name {
		case "generic":
			s.adapters = append(s.adapters, generic.New(home))
		case "files":
			s.adapters = append(s.adapters, files.New(home))
		case "git":
			s.git = true
		default:
			return s, fmt.Errorf("unknown adapter %q", name)
		}
	}
	return s, nil
}

// includes runs the separate Git include step after apply, as KIT-23's
// sync will: "include <path>" for each added include, warnings on stderr.
func (s selection) includes(host platform.Host, repo, home string, m *manifest.Manifest, stdout, stderr io.Writer) int {
	if !s.git || m.Git == nil {
		return int(apply.OK)
	}
	ctx := context.Background()
	backend, err := system.New(ctx, system.Options{Env: host, NoSystemConfig: true})
	if err != nil {
		return driverError(stderr, err)
	}
	paths := make([]string, 0, len(m.Git.Include))
	for _, include := range m.Git.Include {
		paths = append(paths, filepath.Join(repo, filepath.FromSlash(include)))
	}
	result, err := vcs.EnsureIncludes(ctx, backend, host, home, paths)
	if printErr := printIncludes(stdout, stderr, result); printErr != nil {
		return driverError(stderr, printErr)
	}
	if err != nil {
		return driverError(stderr, err)
	}
	if len(result.Warnings) > 0 {
		return int(apply.Warnings)
	}
	return int(apply.OK)
}

func validDriverArgs(options driverOptions) bool {
	return len(options.args) >= 1 && (options.adapter == "" || options.adapters == "") &&
		(options.adapter != "" || options.adapters != "" || len(options.args) >= 2)
}

func driverSelection(host platform.Host, home string, options driverOptions) (selection, error) {
	if options.adapters != "" {
		return driverAdapters(options.adapters, home, fixtureAdapter(options.args[1:]))
	}
	adapter, err := driverAdapter(host, options.adapter, options.args[1:])
	if err != nil {
		return selection{}, err
	}
	return selection{adapters: []target.Adapter{adapter}}, nil
}

func printIncludes(stdout, stderr io.Writer, result vcs.IncludeResult) error {
	for _, path := range result.Added {
		if _, err := fmt.Fprintln(stdout, "include", path); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintln(stderr, warning); err != nil {
			return err
		}
	}
	return nil
}
