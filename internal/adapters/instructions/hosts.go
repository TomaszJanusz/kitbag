package instructions

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// hostPlaceholder is the instructions.hosts template variable.
const hostPlaceholder = "{host}"

// HostAdditions lists, sorted, the hosts that have a host addition in the
// repository: for a template such as "hosts/{host}.md", every regular file
// "hosts/<name>.md". Only the directory holding the first {host} element and the
// matching entries are read; a template with {host} twice in one element, or
// a missing directory, yields no hosts. It never guesses which host is current.
func HostAdditions(fsys manifest.FileSystem, repository, template string) ([]string, error) {
	elements := strings.Split(template, "/")
	index := slices.IndexFunc(elements, func(e string) bool { return strings.Contains(e, hostPlaceholder) })
	if index < 0 {
		return nil, nil
	}
	prefix, suffix, _ := strings.Cut(elements[index], hostPlaceholder)
	if strings.Contains(suffix, hostPlaceholder) {
		return nil, nil
	}
	entries, err := fsys.ReadDir(join(repository, path.Join(elements[:index]...)))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, platform.ErrNotDir) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, entry := range entries {
		name, ok := hostName(entry.Name(), prefix, suffix)
		if !ok {
			continue
		}
		kind, kindErr := lstat(fsys, join(repository, strings.ReplaceAll(template, hostPlaceholder, name)))
		if kindErr != nil {
			return nil, kindErr
		}
		if kind == regularFile {
			hosts = append(hosts, name)
		}
	}
	slices.Sort(hosts)
	return hosts, nil
}

// hostName extracts the non-empty host between prefix and suffix.
func hostName(entry, prefix, suffix string) (string, bool) {
	name, hasPrefix := strings.CutPrefix(entry, prefix)
	name, hasSuffix := strings.CutSuffix(name, suffix)
	return name, hasPrefix && hasSuffix && name != ""
}

// HostMismatch explains a host taken from the system hostname (host is not
// configured) that has no addition for template while other hosts have one.
// It names their files and the command to choose one, but never picks a host:
// only the maintainer knows which file belongs to this machine. It returns ""
// when the addition exists, the template is empty or no other host has one.
func HostMismatch(fsys manifest.FileSystem, repository, template, host string) (string, error) {
	if template == "" {
		return "", nil
	}
	addition := strings.ReplaceAll(template, hostPlaceholder, host)
	kind, err := lstat(fsys, join(repository, addition))
	if err != nil || kind != missingFile {
		return "", err
	}
	hosts, err := HostAdditions(fsys, repository, template)
	if err != nil || len(hosts) == 0 {
		return "", err
	}
	files := make([]string, 0, len(hosts))
	for _, name := range hosts {
		files = append(files, strings.ReplaceAll(template, hostPlaceholder, name))
	}
	return fmt.Sprintf("host %q is the system hostname (host is not configured) and has no host addition %s, "+
		"so instructions omit every host addition; the repository has %s for host(s) %s: "+
		"if this machine is one of them, run kitbag config set host <name>",
		host, addition, strings.Join(files, ", "), strings.Join(hosts, ", ")), nil
}
