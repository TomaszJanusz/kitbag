package embedded

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	format "github.com/go-git/go-git/v5/plumbing/format/config"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// configs reads explicit locations only; go-git's ConfigScoped would read the
// process HOME and environment rather than the injected platform environment.
func (b *Backend) configs(r *git.Repository) ([]*config.Config, error) {
	paths := []string{}
	if !b.opts.NoSystemConfig {
		paths = append(paths, "/etc/gitconfig")
	}
	if b.opts.Env != nil {
		home, err := b.opts.Env.HomeDir()
		if err != nil {
			return nil, err
		}
		xdg, _ := b.opts.Env.LookupEnv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		paths = append(paths, filepath.Join(xdg, "git", "config"), filepath.Join(home, ".gitconfig"))
	}
	configs := make([]*config.Config, 0, len(paths)+1)
	for _, path := range paths {
		cfg, err := readConfig(path)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	if r != nil {
		cfg, err := r.Config()
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

func readConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: explicit system, XDG and HOME Git config paths; no repository path input.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	cfg := config.NewConfig()
	if len(data) > 0 {
		if err = cfg.Unmarshal(data); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func unsupported(feature string) error { return &vcs.UnsupportedError{Feature: feature} }

func checkConfigs(configs []*config.Config, signing bool) error {
	for _, cfg := range configs {
		for _, section := range cfg.Raw.Sections {
			if err := checkSection(section, signing); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSection(s *format.Section, signing bool) error {
	name := strings.ToLower(s.Name)
	if name == "include" || name == "includeif" {
		return unsupported("Git config includes")
	}
	if name == "merge" && len(s.Subsections) > 0 {
		return unsupported("merge drivers")
	}
	if err := checkOptions(name, s.Options, signing); err != nil {
		return err
	}
	for _, sub := range s.Subsections {
		if err := checkOptions(name, sub.Options, signing); err != nil {
			return err
		}
	}
	return nil
}

func checkOptions(section string, options format.Options, signing bool) error {
	for _, opt := range options {
		key := strings.ToLower(opt.Key)
		if signing && section == "commit" && key == "gpgsign" && enabled(opt.Value) {
			return unsupported("signed commits")
		}
		if feature := unsupportedOption(section+"."+key, opt.Value); feature != "" {
			return unsupported(feature)
		}
	}
	return nil
}

func unsupportedOption(key, value string) string {
	switch key {
	case "url.insteadof", "url.pushinsteadof":
		return "insteadOf protocol rewrites"
	case "http.sslcert", "http.sslkey":
		return "TLS client certificates"
	case "core.sshcommand":
		return "core.sshCommand"
	case "rerere.enabled":
		if enabled(value) {
			return "rerere"
		}
	case "http.emptyauth":
		if enabled(value) {
			return "Kerberos authentication"
		}
	}
	return ""
}

func enabled(value string) bool {
	switch strings.ToLower(value) {
	case "false", "no", "off", "0":
		return false
	}
	return true
}

func (b *Backend) identity(configs []*config.Config) (vcs.Identity, error) {
	id := b.opts.Identity
	for _, cfg := range configs {
		if cfg.User.Name != "" {
			id.Name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			id.Email = cfg.User.Email
		}
	}
	if strings.TrimSpace(id.Name) == "" || strings.TrimSpace(id.Email) == "" {
		return id, unsupported("commit identity missing; set git.identity.name and git.identity.email in config.toml")
	}
	return id, nil
}
