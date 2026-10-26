package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// RefreshRequest supplies the read-only API observation of a running daemon.
type RefreshRequest struct {
	DaemonVersion, BinaryVersion string
	Running, DryRun              bool
}

// RefreshResult is service_schema=1, including the proposed work in dry runs.
type RefreshResult struct {
	DaemonVersion     string `json:"daemon_version,omitempty"`
	BinaryVersion     string `json:"binary_version"`
	Installed         bool   `json:"installed"`
	DefinitionChanged bool   `json:"definition_changed"`
	RestartNeeded     bool   `json:"restart_needed"`
	Changed           bool   `json:"changed"`
	DryRun            bool   `json:"dry_run"`
	ServiceSchema     int    `json:"service_schema"`
}

// desired retains the installer's environment, mode, cadence and config rather
// than replacing them with a different calling shell's settings.
func (s Service) desired(layout platform.ServiceLayout, saved Options) (Options, error) {
	saved.Binary = layout.Binary
	if layout.Manager == "launchd" {
		if saved.Environment == nil {
			saved.Environment = map[string]string{}
		}
		saved.Environment["PATH"] = s.daemonPath(saved.Environment["PATH"])
	}
	return saved, saved.validate()
}

// Refresh reconciles an existing installation. No installation, enablement or
// activation is introduced. Definition replacement precedes any stop, so a
// write failure leaves the loaded service running. Repeating it does no work.
func (s Service) Refresh(ctx context.Context, request RefreshRequest) (r RefreshResult, err error) {
	r = RefreshResult{ServiceSchema: 1, DaemonVersion: request.DaemonVersion, BinaryVersion: request.BinaryVersion, DryRun: request.DryRun}
	installed, err := s.Installed()
	if err != nil || !installed {
		return r, err
	}
	if request.DryRun {
		return s.refresh(ctx, request)
	}
	lock, err := s.refreshLock()
	if err != nil {
		return r, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return s.refresh(ctx, request)
}

func (s Service) refresh(ctx context.Context, request RefreshRequest) (RefreshResult, error) {
	r := RefreshResult{ServiceSchema: 1, DaemonVersion: request.DaemonVersion, BinaryVersion: request.BinaryVersion, DryRun: request.DryRun}
	report, err := s.Status(ctx)
	r.Installed = report.Installed
	if err != nil || !r.Installed {
		return r, err
	}
	report, err = s.resumeRefresh(report)
	if err != nil {
		return r, err
	}
	request = runningVersion(report, request)
	r.DaemonVersion = request.DaemonVersion
	r.DefinitionChanged = report.Stale
	r.RestartNeeded = needsRestart(report, request)
	r.Changed = r.DefinitionChanged || r.RestartNeeded
	if request.DryRun || !r.Changed {
		return r, nil
	}
	return r, s.refreshInstalled(ctx, report, r)
}

func needsRestart(report Report, request RefreshRequest) bool {
	mismatch := report.Mode == "daemon" && request.Running && request.DaemonVersion != request.BinaryVersion
	return report.Active && (report.Stale || mismatch)
}

func (s Service) refreshInstalled(ctx context.Context, report Report, r RefreshResult) error {
	if err := s.savePending(report); err != nil {
		return err
	}
	if r.DefinitionChanged {
		if err := s.rewrite(); err != nil {
			return err
		}
	}
	if err := s.Host.ServiceControl(ctx, "reload", false); err != nil {
		return err
	}
	if report.Manager == "launchd" {
		if err := s.refreshLaunchd(ctx, report); err != nil {
			return err
		}
	} else if r.RestartNeeded {
		if err := s.Host.ServiceControl(ctx, "restart", report.Mode == "timer"); err != nil {
			return err
		}
	}
	return s.clearPending()
}

func (s Service) refreshLaunchd(ctx context.Context, report Report) error {
	current, err := s.Host.ServiceInspect(ctx, report.Mode == "timer")
	if err != nil {
		return err
	}
	if current.Loaded {
		if err = s.Host.ServiceControl(ctx, "stop", report.Mode == "timer"); err != nil {
			return err
		}
	}
	if report.Loaded || report.Active {
		return s.Host.ServiceControl(ctx, "start", report.Mode == "timer")
	}
	return nil
}

func (s Service) rewrite() error {
	layout, err := s.Host.ServiceLayout()
	if err != nil {
		return err
	}
	saved, err := s.saved()
	if err != nil {
		return fmt.Errorf("saved service inputs unavailable; run kitbag service install: %w", err)
	}
	options, err := s.desired(layout, saved)
	if err != nil {
		return err
	}
	files, err := Generate(layout.Manager, options)
	if err != nil {
		return err
	}
	return s.write(layout, files, options)
}

// AutoRefreshInterval bounds startup repair attempts, including failed reloads.
const AutoRefreshInterval = 10 * time.Minute

type refreshMarker struct {
	Attempts map[string]time.Time `json:"attempts"`
}

// AutoRefresh checks only this daemon's own saved installation. The marker is
// durable before any supervisor action. systemd reloads without killing the
// caller; restart=true asks the caller to exit 75 after releasing its locks.
// launchd delegates the complete rewrite/bootout/bootstrap to detach, which
// must run outside the daemon's process group. The caller then keeps serving.
func (s Service) AutoRefresh(ctx context.Context, version string, now time.Time, detach func() error) (bool, error) {
	_, needed, err := s.autoRefreshReport(ctx)
	if err != nil || !needed {
		return false, err
	}
	restart, helper, err := s.prepareAutoRefresh(ctx, version, now)
	if err != nil || !helper {
		return restart, err
	}
	return false, detach()
}

// Release the mutation lock before starting launchd's helper, which acquires
// that same lock. Recheck the snapshot after acquiring it to avoid duplicate
// repair when an explicit refresh completed in the meantime.
func (s Service) prepareAutoRefresh(ctx context.Context, version string, now time.Time) (restart, helper bool, err error) {
	lock, err := s.refreshLock()
	if err != nil {
		return false, false, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	report, needed, err := s.autoRefreshReport(ctx)
	if err != nil || !needed {
		return false, false, err
	}
	dirs, err := s.Host.Dirs()
	if err != nil {
		return false, false, err
	}
	allowed, err := s.allowAutoRefresh(filepath.Join(dirs.State, "service-refresh.json"), version, now)
	if err != nil || !allowed {
		return false, false, err
	}
	return s.stageAutoRefresh(ctx, report)
}

func (s Service) stageAutoRefresh(ctx context.Context, report Report) (bool, bool, error) {
	if err := s.savePending(report); err != nil {
		return false, false, err
	}
	if report.Manager == "launchd" {
		return false, true, nil
	}
	if err := s.rewrite(); err != nil {
		return false, false, err
	}
	err := s.Host.ServiceControl(ctx, "reload", false)
	if err == nil {
		err = s.clearPending()
	}
	return err == nil, false, err
}

func (s Service) autoRefreshReport(ctx context.Context) (Report, bool, error) {
	report, err := s.Status(ctx)
	if err != nil || !report.Installed || !report.Stale || !report.Active {
		return report, false, err
	}
	saved, err := s.saved()
	if err != nil {
		return report, false, err
	}
	config := s.Config
	if config == "" {
		dirs, dirErr := s.Host.Dirs()
		if dirErr != nil {
			return report, false, dirErr
		}
		config = filepath.Join(dirs.Config, "config.toml")
	}
	return report, saved.Mode == "daemon" && filepath.Clean(saved.Config) == filepath.Clean(config), nil
}

func (s Service) allowAutoRefresh(path, version string, now time.Time) (bool, error) {
	data, err := s.Host.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	previous := refreshMarker{Attempts: map[string]time.Time{}}
	if err == nil {
		if err = json.Unmarshal(data, &previous); err != nil {
			return false, err
		}
		if at, ok := previous.Attempts[version]; ok && now.Sub(at) < AutoRefreshInterval {
			return false, nil
		}
	}
	previous.record(version, now)
	data, err = json.Marshal(previous)
	if err != nil {
		return false, err
	}
	return true, s.Host.WriteAtomic(path, data, privatePerm)
}

func (previous *refreshMarker) record(version string, now time.Time) {
	if previous.Attempts == nil {
		previous.Attempts = map[string]time.Time{}
	}
	for v, at := range previous.Attempts {
		if now.Sub(at) >= AutoRefreshInterval {
			delete(previous.Attempts, v)
		}
	}
	previous.Attempts[version] = now
}
