package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// RecoveryWindow bounds the transient notice after requesting a supervisor
// restart. Older daemons can take two minute-long observations to exit.
const RecoveryWindow = 150 * time.Second

type versionRecovery struct {
	At      time.Time `json:"at"`
	Version string    `json:"version"`
	Error   string    `json:"error,omitempty"`
}

// RecoverVersion runs in the installed client, never in the daemon being
// replaced. It repairs only an enabled, saved daemon installation for this
// config. A durable attempt marker prevents status polling or multiple clients
// from causing restart loops. Pending is bounded even if the supervisor lies.
func (s Service) RecoverVersion(ctx context.Context, request RefreshRequest, now time.Time) (pending bool, err error) {
	if !newerBinary(request) || now.IsZero() {
		return false, nil
	}
	report, err := s.Status(ctx)
	if err != nil || !recoverable(report, request) {
		return false, err
	}
	allowed, err := s.recoveryInputs()
	if err != nil || !allowed {
		return false, err
	}
	lock, err := s.refreshLock()
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return s.recoverVersion(ctx, request, now)
}

func (s Service) recoveryInputs() (bool, error) {
	current, err := s.Current("daemon", DefaultInterval)
	if err != nil {
		return false, err
	}
	saved, err := s.saved()
	if err != nil || filepath.Clean(current.Config) != filepath.Clean(saved.Config) {
		return false, err
	}
	info, err := s.Host.Stat(current.Binary)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return false, fs.ErrInvalid
	}
	return true, nil
}

func newerBinary(r RefreshRequest) bool {
	if r.DryRun || r.BinaryVersion == "" || r.BinaryVersion == r.DaemonVersion {
		return false
	}
	if err := (&manifest.Manifest{}).CheckVersion(r.BinaryVersion); err != nil {
		return false
	}
	if r.DaemonVersion == "" || r.DaemonVersion == "older" {
		return true
	}
	if err := (&manifest.Manifest{}).CheckVersion(r.DaemonVersion); err != nil {
		return false
	}
	return (&manifest.Manifest{MinToolVersion: r.DaemonVersion}).CheckVersion(r.BinaryVersion) == nil
}

func recoverable(r Report, request RefreshRequest) bool {
	if !r.Installed || !r.Enabled || r.Mode != "daemon" || !recoveryManager(r.Manager) {
		return false
	}
	if r.Active && request.Running {
		return true
	}
	// A loaded launchd job pointing into a removed keg cannot relaunch itself.
	// An intentionally stopped (booted-out) job is not loaded and stays stopped.
	return r.Manager == "launchd" && r.Loaded && r.Stale && r.BinaryMissing
}

func (s Service) recoverVersion(ctx context.Context, request RefreshRequest, now time.Time) (bool, error) {
	dirs, err := s.Host.Dirs()
	if err != nil {
		return false, err
	}
	path := filepath.Join(dirs.State, "service-version-recovery.json")
	pending, recent, err := s.recentVersionRecovery(path, request.BinaryVersion, now)
	if err != nil || recent {
		return pending, err
	}
	record := versionRecovery{Version: request.BinaryVersion, At: now}
	if err = s.saveVersionRecovery(path, record); err != nil {
		return false, err
	}
	result, err := s.refresh(ctx, request)
	if err != nil {
		record.Error = err.Error()
		return false, errors.Join(err, s.saveVersionRecovery(path, record))
	}
	return result.Changed, nil
}

func (s Service) saveVersionRecovery(path string, record versionRecovery) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.Host.WriteAtomic(path, data, privatePerm)
}

func (s Service) recentVersionRecovery(path, version string, now time.Time) (bool, bool, error) {
	var record versionRecovery
	data, err := s.Host.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return false, false, err
	}
	if now.Before(record.At) {
		return false, true, errors.New("automatic service recovery timestamp is in the future")
	}
	if record.Version != version || now.Sub(record.At) >= AutoRefreshInterval {
		return false, false, nil
	}
	if record.Error != "" {
		return false, true, fmt.Errorf("previous automatic service recovery failed: %s", record.Error)
	}
	return now.Sub(record.At) < RecoveryWindow, true, nil
}

func recoveryManager(manager string) bool { return manager == "launchd" || manager == "systemd" }
