// Package sshauth resolves embedded SSH credentials using explicit host inputs.
package sshauth

import (
	"bufio"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

const defaultPort = 22

// Target contains resolved connection settings, never private key bytes.
type Target struct {
	Host, User, Agent      string
	home                   string
	Identities, KnownHosts []string
	Port                   int
	IdentitiesOnly         bool
}

// Resolve applies URL values before SSH configuration, then defaults.
func Resolve(env platform.Env, files platform.FS, host, user string, port int, system bool) (Target, error) {
	if env == nil || files == nil {
		return Target{}, vcs.ErrInvalidArgument
	}
	home, err := env.HomeDir()
	if err != nil {
		return Target{}, err
	}
	paths := []string{filepath.Join(home, ".ssh", "config")}
	if system {
		paths = append(paths, "/etc/ssh/ssh_config")
	}
	cfg, err := readConfig(files, paths)
	if err != nil {
		return Target{}, err
	}
	t := Target{Host: strings.Trim(host, "[]"), User: user, Port: port, home: home}
	values := configValues(cfg, host)
	if err = t.connection(values, env); err != nil {
		return Target{}, err
	}
	return t.resolvePaths(cfg, host, values["userknownhostsfile"])
}

func configValues(cfg *ssh_config.Config, alias string) map[string]string {
	values := map[string]string{}
	for _, host := range cfg.Hosts {
		if !host.Matches(alias) {
			continue
		}
		for _, node := range host.Nodes {
			if entry, ok := node.(*ssh_config.KV); ok {
				key := strings.ToLower(entry.Key)
				if _, exists := values[key]; !exists {
					values[key] = entry.Value
				}
			}
		}
	}
	return values
}

func (t *Target) connection(values map[string]string, env platform.Env) error {
	if name := values["hostname"]; name != "" {
		t.Host = strings.ReplaceAll(name, "%h", t.Host)
	}
	if t.User == "" {
		t.User = values["user"]
	}
	if t.User == "" {
		t.User = "git"
	}
	if err := t.resolvePort(values["port"]); err != nil {
		return err
	}
	t.IdentitiesOnly = strings.EqualFold(values["identitiesonly"], "yes")
	t.Agent = values["identityagent"]
	if t.Agent == "" || t.Agent == "SSH_AUTH_SOCK" {
		t.Agent, _ = env.LookupEnv("SSH_AUTH_SOCK")
	}
	if t.Agent == "none" {
		t.Agent = ""
	}
	return nil
}

func (t *Target) resolvePort(value string) error {
	if t.Port == 0 && value != "" {
		var err error
		t.Port, err = strconv.Atoi(value)
		if err != nil {
			return errors.New("invalid SSH Port")
		}
	}
	if t.Port == 0 {
		t.Port = defaultPort
	}
	const maxPort = 65535
	if t.Port < 1 || t.Port > maxPort {
		return errors.New("invalid SSH Port")
	}
	return nil
}

func (t Target) resolvePaths(cfg *ssh_config.Config, alias, known string) (Target, error) {
	identities, err := cfg.GetAll(alias, "IdentityFile")
	if err != nil {
		return t, err
	}
	// Like OpenSSH, default keys are tried only when no IdentityFile is set.
	if len(identities) == 0 {
		identities = append(identities, "~/.ssh/id_ed25519", "~/.ssh/id_ecdsa", "~/.ssh/id_rsa")
	}
	if err = t.identityPaths(identities); err != nil {
		return t, err
	}

	if err = t.knownPaths(known); err != nil {
		return t, err
	}
	if t.Agent != "" {
		t.Agent, err = t.expand(t.Agent)
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

func (t *Target) knownPaths(known string) error {
	if known == "" {
		known = "~/.ssh/known_hosts"
	}
	paths, err := pathWords(known)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if path != "none" {
			expanded, expandErr := t.expand(path)
			if expandErr != nil {
				return expandErr
			}
			t.KnownHosts = append(t.KnownHosts, expanded)
		}
	}
	if len(t.KnownHosts) == 0 {
		return errors.New("SSH requires a UserKnownHostsFile; configure known_hosts")
	}
	return nil
}

func (t *Target) identityPaths(identities []string) error {
	for _, identity := range identities {
		if identity == "none" {
			continue
		}
		path, err := t.expand(identity)
		if err != nil {
			return err
		}
		if !slices.Contains(t.Identities, path) {
			t.Identities = append(t.Identities, path)
		}
	}
	return nil
}

func (t Target) expand(path string) (string, error) {
	path = strings.Trim(path, "\"")
	path = strings.NewReplacer("%%", "\x00", "%d", t.home, "%h", t.Host, "%r", t.User, "%p", strconv.Itoa(t.Port)).Replace(path)
	if strings.Contains(path, "%") {
		return "", unsupported("SSH path expansion")
	}
	path = strings.ReplaceAll(path, "\x00", "%")
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(t.home, path[2:])
	}
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		path = filepath.Join(t.home, path)
	}
	if !filepath.IsAbs(path) {
		return "", unsupported("SSH path expansion")
	}
	return filepath.Clean(path), nil
}

// pathWords accepts quoted paths without performing shell or environment expansion.
func pathWords(value string) ([]string, error) {
	var paths []string
	start := -1
	quoted := false
	for i, char := range value {
		if char == '"' {
			quoted = !quoted
		}
		if (char == ' ' || char == '\t') && !quoted {
			if start >= 0 {
				paths = append(paths, strings.Trim(value[start:i], "\""))
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if quoted {
		return nil, errors.New("invalid quoted SSH path")
	}
	if start >= 0 {
		paths = append(paths, strings.Trim(value[start:], "\""))
	}
	return paths, nil
}

func readConfig(files platform.FS, paths []string) (*ssh_config.Config, error) {
	var parts []string
	for _, path := range paths {
		data, err := files.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errors.New("cannot read SSH config; check its permissions")
		}
		if err = CheckConfig(data); err != nil {
			return nil, err
		}
		parts = append(parts, string(data))
	}
	cfg, err := ssh_config.Decode(strings.NewReader(strings.Join(parts, "\nHost *\n")))
	if err != nil {
		return nil, errors.New("cannot parse SSH config; repair it or install git")
	}
	return cfg, nil
}

// CheckConfig rejects unsupported directives before the parser can follow includes.
// Detection remains conservative across Host blocks, matching the embedded policy.
func CheckConfig(data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(strings.ReplaceAll(scanner.Text(), "=", " "))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "proxyjump", "proxycommand", "match", "include", "gssapiauthentication":
			return unsupported("SSH " + fields[0])
		}
	}
	return scanner.Err()
}

func unsupported(feature string) error { return &vcs.UnsupportedError{Feature: feature} }

func (Target) label(path string) string { return "identity file " + filepath.Base(path) }
