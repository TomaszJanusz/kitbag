package doctor

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	probeDirPerm  fs.FileMode = 0o700
	probeFilePerm fs.FileMode = 0o600
)

func (s Service) materialChecks() []Check {
	dirs, err := s.Host.Dirs()
	if err != nil {
		return []Check{result("materialization", "cannot resolve probe directory", "repair XDG directory configuration", err, Fail)}
	}
	copyErr, linkErr := probeMaterialization(s.Host, dirs.Cache)
	envMode, _ := s.Host.LookupEnv("KITBAG_MODE")
	mode, modeErr := apply.ResolveMode("", s.ModeFlag, envMode, s.Config.Sync.Mode, "")
	severity := Fail
	if mode == "copy" {
		severity = Warn
	}
	return []Check{
		result("materialization-mode", "materialization mode "+mode, "select --mode=link or --mode=copy", modeErr, Fail),
		result("copy", "atomic copy available in cache filesystem", "make the cache directory writable and use a filesystem supporting atomic writes", copyErr, Fail),
		result("link", "symbolic links available in cache filesystem", "use --mode=copy or enable symbolic links on the filesystem", linkErr, severity),
	}
}

// Probe only unpredictable, newly created names. Never overwrite existing data.
// This tests the cache filesystem, not every possible destination mount.
func probeMaterialization(host platform.FS, cache string) (copyErr, linkErr error) {
	path := filepath.Join(cache, "doctor-"+rand.Text())
	if _, err := host.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		return fs.ErrExist, fs.ErrExist
	}
	if err := host.MkdirAll(path, probeDirPerm); err != nil {
		return err, err
	}
	source, link := filepath.Join(path, "copy"), filepath.Join(path, "link")
	defer func() {
		linkErr = errors.Join(linkErr, removeProbe(host, link))
		copyErr = errors.Join(copyErr, removeProbe(host, source), host.Remove(path))
	}()
	payload := []byte("kitbag doctor\n")
	copyErr = host.WriteAtomic(source, payload, probeFilePerm)
	if copyErr == nil {
		data, err := host.ReadFile(source)
		copyErr = err
		if err == nil && !bytes.Equal(data, payload) {
			copyErr = fs.ErrInvalid
		}
	}
	linkErr = host.Link(source, link)
	if linkErr == nil {
		target, err := host.ReadLink(link)
		linkErr = err
		if err == nil && target != source {
			linkErr = fs.ErrInvalid
		}
	}
	return copyErr, linkErr
}

func removeProbe(host platform.FS, path string) error {
	err := host.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
