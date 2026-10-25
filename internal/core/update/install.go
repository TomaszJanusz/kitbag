package update

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

func errorsIsMissing(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// install serializes update writers, preserves the old executable without ever
// removing its live name, and uses the platform's fsync + rename writer.
func (s Service) install(ctx context.Context, release artifact) (err error) {
	lock, err := s.FS.TryLock(s.Binary + ".update.lock")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	info, err := s.FS.Lstat(s.Binary)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("binary must be a regular file: %w", fs.ErrInvalid)
	}
	// Re-read provenance while holding the update lock.
	source, err := Source(s.FS, s.Binary)
	if err != nil || source != "install-script" && source != "release-archive" {
		return fmt.Errorf("install ownership changed: %w", errors.Join(err, fs.ErrPermission))
	}
	if _, err = s.FS.Lstat(s.Binary + ".old"); !errorsIsMissing(err) {
		return fmt.Errorf("previous update needs inspection (.old exists): %w", errors.Join(err, fs.ErrExist))
	}
	return s.installChecked(ctx, release, info.Mode().Perm())
}

func (s Service) replaceAndConfirm(ctx context.Context, release artifact, mode fs.FileMode) error {
	old, err := s.FS.ReadFile(s.Binary)
	if err != nil {
		return err
	}
	if err = s.FS.WriteAtomic(s.Binary+".old", old, mode); err != nil {
		return err
	}
	if err = s.FS.WriteAtomic(s.Binary, release.binary, binaryPerm); err != nil {
		return errors.Join(err, s.restore(old, mode))
	}
	version, probeErr := s.Probe(ctx, s.Binary)
	if probeErr != nil || version != "kitbag "+release.version {
		return errors.Join(fmt.Errorf("updated binary version confirmation failed: %w", errors.Join(probeErr, fs.ErrInvalid)), s.restore(old, mode))
	}
	return s.FS.Remove(s.Binary + ".old")
}

func (s Service) restore(old []byte, mode fs.FileMode) error {
	if err := s.FS.WriteAtomic(s.Binary, old, mode); err != nil {
		return fmt.Errorf("rollback failed; original retained at .old: %w", err)
	}
	return s.FS.Remove(s.Binary + ".old")
}

// Another updater may have replaced the executable while this process downloaded.
// Compare the executable now installed, as well as the running process version.
func (s Service) installChecked(ctx context.Context, release artifact, mode fs.FileMode) error {
	output, err := s.Probe(ctx, s.Binary)
	if err != nil {
		return fmt.Errorf("installed binary version probe: %w", err)
	}
	current := strings.TrimPrefix(output, "kitbag ")
	if current == "dev" {
		current = "0.0.0-dev"
	}
	if err = newer(current, release.version); err != nil {
		return err
	}
	return s.replaceAndConfirm(ctx, release, mode)
}
