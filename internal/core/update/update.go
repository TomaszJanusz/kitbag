// Package update implements verified self-update policy with injected I/O.
package update

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Fetch downloads a bounded resource; production transport lives outside core.
type Fetch func(context.Context, string, int64) ([]byte, error)

// Service supplies host, transport, signature and process dependencies.
type Service struct {
	FS                                 platform.FS
	Fetch                              Fetch
	Verify                             func(checksums, signature []byte) error
	Probe                              func(context.Context, string) (string, error)
	Exec                               func(context.Context, string, []string) error
	Binary, Current, OS, Arch, BaseURL string
}

// Options select a channel and whether to stop after verifying the download.
type Options struct {
	Channel string
	Check   bool
}

// Result describes update availability or the install owner's instructions.
type Result struct {
	Source, Version, Command string
	Updated                  bool
}

const binaryPerm fs.FileMode = 0o755

// Run never replaces package-owned or unstamped binaries. Check verifies the
// complete release without changing disk. Versions can only move forward.
func (s Service) Run(ctx context.Context, options Options) (Result, error) {
	var result Result
	if options.Channel != "stable" && options.Channel != "nightly" {
		return result, fmt.Errorf("channel must be stable or nightly: %w", fs.ErrInvalid)
	}
	source, err := Source(s.FS, s.Binary)
	result.Source, result.Command = source, Instructions(source)
	if err != nil || source != "install-script" && source != "release-archive" {
		return result, err
	}
	if s.Fetch == nil || s.Verify == nil {
		return result, fmt.Errorf("missing release verifier or transport: %w", fs.ErrInvalid)
	}
	return s.direct(ctx, options, result)
}

func (s Service) direct(ctx context.Context, options Options, result Result) (Result, error) {
	release, err := s.download(ctx, options.Channel)
	if err != nil {
		return result, err
	}
	result.Version = release.version
	if err = newer(s.Current, release.version); err != nil {
		return result, err
	}
	result.Command = "kitbag is up to date"
	if s.Current == release.version || options.Check {
		return result, nil
	}
	if s.Probe == nil || s.Exec == nil {
		return result, fmt.Errorf("missing process operations: %w", fs.ErrInvalid)
	}
	if err = s.install(ctx, release); err != nil {
		return result, err
	}
	result.Updated, result.Command = true, "updated kitbag to "+release.version
	return result, s.Exec(ctx, s.Binary, []string{"version"})
}

func newer(current, next string) error {
	if err := (&manifest.Manifest{}).CheckVersion(current); err != nil {
		return err
	}
	if err := (&manifest.Manifest{MinToolVersion: current}).CheckVersion(next); err != nil {
		return fmt.Errorf("refusing downgrade or invalid release version: %w", err)
	}
	return nil
}

// Source reads the install-time sibling stamp through the platform filesystem.
// Missing and unrecognized stamps conservatively return unknown.
func Source(host platform.FS, binary string) (string, error) {
	if !filepath.IsAbs(binary) || filepath.Clean(binary) != binary {
		return "unknown", fs.ErrInvalid
	}
	data, err := host.ReadFile(filepath.Join(filepath.Dir(binary), "kitbag.install-source"))
	if errorsIsMissing(err) {
		return "unknown", nil
	}
	if err != nil {
		return "unknown", err
	}
	source := strings.TrimSpace(string(data))
	switch source {
	case "homebrew", "deb", "rpm", "aur", "install-script", "release-archive", "go-install":
		return source, nil
	default:
		return "unknown", nil
	}
}

// Instructions return the owning installer's command, never executing it.
func Instructions(source string) string {
	switch source {
	case "homebrew":
		return "brew upgrade kitbag"
	case "deb":
		return "sudo apt-get update && sudo apt-get install --only-upgrade kitbag"
	case "rpm":
		return "sudo dnf upgrade kitbag"
	case "aur":
		return "update kitbag with your AUR helper (for example: yay -Syu kitbag)"
	case "go-install":
		return "go install github.com/TomaszJanusz/kitbag/cmd/kitbag@latest"
	default:
		return "install source unknown; reinstall kitbag through its original installer"
	}
}
