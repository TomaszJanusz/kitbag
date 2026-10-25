package initialize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Template holds an agent selection for a new repository. Instances use
// agent.name=~/directory; existing repositories retain their own target policy.
type Template struct{ Agents, Instances []string }

type templateFile struct {
	name string
	data []byte
}

const (
	directoryPerm fs.FileMode = 0o700
	filePerm      fs.FileMode = 0o600
)

// Manifest renders the minimal example's instruction layout with explicit
// selected targets. Parse enforces the same grammar as a hand-written manifest.
func (t Template) Manifest() ([]byte, error) {
	text := []string{"schema = 1\n\n[instructions]\nsource = \"AGENTS.md\"\n"}
	seen := map[string]bool{}
	for _, agent := range t.Agents {
		if (agent != "claude" && agent != "codex") || seen[agent] {
			return nil, fmt.Errorf("unsupported or duplicate scaffold agent %q: %w", agent, fs.ErrInvalid)
		}
		seen[agent] = true
		text = append(text, fmt.Sprintf("\n[targets.%s]\nagent = %q\n", agent, agent))
	}
	instances, err := t.renderInstances(seen)
	if err != nil {
		return nil, err
	}
	data := []byte(strings.Join(append(text, instances...), ""))
	_, err = manifest.Parse("kitbag.toml", data)
	return data, err
}

func (t Template) renderInstances(agents map[string]bool) ([]string, error) {
	text := make([]string, 0, len(t.Instances))
	instances := map[string]bool{}
	for _, instance := range t.Instances {
		key, dir, ok := strings.Cut(instance, "=")
		agent, name, named := strings.Cut(key, ".")
		if !ok || !named || name == "" || !agents[agent] || instances[key] {
			return nil, fmt.Errorf("instance must be selected-agent.name=~/directory: %w", fs.ErrInvalid)
		}
		instances[key] = true
		text = append(text, fmt.Sprintf("\n[targets.%s.instances.%s]\ndir = %q\n", agent, name, dir))
	}
	return text, nil
}

// Scaffold creates the minimal example repository. Preflight refuses any
// existing template file before writing, and a completed template is idempotent.
// No real agent directory, machine configuration or remote is changed.
func Scaffold(ctx context.Context, host platform.Host, backend vcs.Backend, dir string, template Template) (bool, error) {
	data, err := template.Manifest()
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return false, fs.ErrInvalid
	}
	files := []templateFile{
		{"kitbag.toml", data},
		{"README.md", []byte("# Agent configuration\n\nEdit AGENTS.md for shared instructions. Targets and instances live in kitbag.toml.\n\nCommit and push this repository, then run `kitbag init <git-url>` on each machine.\nReview `kitbag plan` before `kitbag sync`. Service installation arrives in M2.\n")},
		{"AGENTS.md", []byte("# Shared agent instructions\n\nDescribe project conventions and safe working practices here.\n")},
	}
	complete, err := scaffoldPreflight(host, dir, filesNames(files))
	if complete || err != nil {
		return complete, err
	}
	return false, writeScaffold(ctx, host, backend, dir, files)
}

func writeScaffold(ctx context.Context, host platform.FS, backend vcs.Backend, dir string, files []templateFile) error {
	if err := host.MkdirAll(dir, directoryPerm); err != nil {
		return err
	}
	initializer, ok := backend.(vcs.Initializer)
	if !ok {
		return fmt.Errorf("repository initialization: %w", platform.ErrUnsupported)
	}
	if err := initializer.Init(ctx, dir); err != nil {
		return err
	}
	for _, file := range files {
		if err := createFile(host, filepath.Join(dir, file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

// Stage then rename through the host's no-replace contract. A competing file
// creation wins rather than being overwritten between preflight and install.
func createFile(host platform.FS, path string, data []byte) (err error) {
	stage := path + ".init-new"
	exists, err := Exists(host, stage)
	if err != nil {
		return err
	}
	if exists {
		return fs.ErrExist
	}
	if err = host.WriteAtomic(stage, data, filePerm); err != nil {
		return err
	}
	defer func() {
		cleanup := host.Remove(stage)
		if !errors.Is(cleanup, fs.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	return host.Rename(stage, path)
}

func filesNames(files []templateFile) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.name)
	}
	return names
}

func scaffoldPreflight(host platform.FS, dir string, names []string) (bool, error) {
	info, err := host.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("scaffold directory: %w", fs.ErrExist)
	}
	count := 0
	for _, name := range names {
		exists, err := Exists(host, filepath.Join(dir, name))
		if err != nil {
			return false, err
		}
		if exists {
			count++
		}
	}
	if count == len(names) {
		return true, nil
	}
	if count != 0 {
		return false, fmt.Errorf("scaffold files already exist: %w", fs.ErrExist)
	}
	return false, nil
}
