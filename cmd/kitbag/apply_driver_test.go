//go:build darwin || linux

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// testKitbagMain adds an entry only to the test binary. Installed kitbag has no
// apply/plan/sync/status command; KIT-23 owns the reviewed public CLI wiring.
func testKitbagMain() {
	if len(os.Args) > 1 && os.Args[1] == "__e2e_apply" {
		os.Exit(applyDriver(os.Args[2:], os.Stdout, os.Stderr))
	}
	main()
}

func applyDriver(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("__e2e_apply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mode := flags.String("mode", "", "machine mode")
	force := flags.Bool("force", false, "discard copy drift after backup")
	if err := flags.Parse(args); err != nil {
		return int(apply.Usage)
	}
	if flags.NArg() < 2 {
		return int(apply.Usage)
	}
	host := unix.New()
	m, err := fixtureManifest(host, flags.Arg(0))
	if err != nil {
		return driverError(stderr, err)
	}
	machineMode, err := apply.ResolveMode("", *mode, "", "", "")
	if err != nil {
		return driverError(stderr, err)
	}
	store, err := state.New(host, host, state.Options{})
	if err != nil {
		return driverError(stderr, err)
	}
	req := plan.Request{Repository: flags.Arg(0), Manifest: m, Adapters: []target.Adapter{fixtureAdapter(flags.Args()[1:])}}
	home, err := host.HomeDir()
	if err != nil {
		return driverError(stderr, err)
	}
	req.LegacyRoots = []string{req.Repository, filepath.Join(home, ".agents", "skills")}
	result, err := apply.Run(host, store, req, apply.Options{Mode: machineMode, Force: *force, Now: time.Now})
	if printErr := printApply(stdout, stderr, result); printErr != nil {
		return driverError(stderr, printErr)
	}
	if err != nil {
		return max(int(result.Code()), driverError(stderr, err))
	}
	return int(result.Code())
}

func fixtureManifest(host platform.Host, repo string) (*manifest.Manifest, error) {
	m, err := manifest.Load(host, filepath.Join(repo, "kitbag.toml"))
	if err != nil {
		return nil, err
	}
	if err = m.CheckVersion("0.1.0"); err != nil {
		return nil, err
	}
	return m.ResolveHost(host, "test")
}

type fixtureAdapter []string

func (instances fixtureAdapter) Declare(fsys manifest.FileSystem, repo string, _ *manifest.Manifest) ([]target.Target, error) {
	entries, err := fsys.ReadDir(filepath.Join(repo, "global"))
	if err != nil {
		return nil, err
	}
	var ds []target.Target
	for _, dir := range instances {
		for _, entry := range entries {
			ds = append(ds, target.Target{Path: filepath.Join(dir, entry.Name()), InstanceDir: dir, SourcePath: filepath.Join(repo, "global", entry.Name()), Kind: target.Materialized, Backup: target.Once, Owners: []state.Owner{{Primitive: "fixture", AgentInstance: dir}}})
		}
	}
	return ds, nil
}

func printApply(stdout, stderr io.Writer, result apply.Result) error {
	for _, op := range result.Changed {
		if _, err := fmt.Fprintln(stdout, op.Kind, op.Path); err != nil {
			return err
		}
	}
	for _, d := range append(result.Warnings, result.Conflicts...) {
		if _, err := fmt.Fprintln(stderr, d.Path+":", d.Message); err != nil {
			return err
		}
	}
	return nil
}

func driverError(stderr io.Writer, err error) int {
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		return int(apply.Warnings)
	}
	return int(apply.ErrorCode(err))
}
