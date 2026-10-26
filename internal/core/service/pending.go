package service

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s Service) refreshLock() (platform.Unlocker, error) {
	dirs, err := s.Host.Dirs()
	if err != nil {
		return nil, err
	}
	if err = s.Host.MkdirAll(dirs.State, directoryPerm); err != nil {
		return nil, err
	}
	return s.Host.TryLock(filepath.Join(dirs.State, "service-refresh.lock"))
}

func (s Service) pendingPath() (string, error) {
	dirs, err := s.Host.Dirs()
	return filepath.Join(dirs.State, "service-refresh-pending.json"), err
}

// A failed reload/bootstrap must remain repairable even after files match the
// generator. Retain the prior activation state until the whole repair succeeds.
func (s Service) savePending(report Report) error {
	path, err := s.pendingPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return s.Host.WriteAtomic(path, data, privatePerm)
}

func (s Service) pending() (Report, bool, error) {
	path, err := s.pendingPath()
	if err != nil {
		return Report{}, false, err
	}
	data, err := s.Host.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	var previous Report
	if err = json.Unmarshal(data, &previous); err != nil {
		return Report{}, false, err
	}
	return previous, true, nil
}

func (s Service) pendingStatus(report Report) (Report, error) {
	_, exists, err := s.pending()
	if err != nil || !exists {
		return report, err
	}
	report.Stale = true
	return report, nil
}

// Only the repair plan restores prior activation intent. Status continues to
// report the supervisor's actual state after a failed bootout/bootstrap.
func (s Service) resumeRefresh(report Report) (Report, error) {
	previous, exists, err := s.pending()
	if err != nil || !exists {
		return report, err
	}
	report.Stale = true
	report.Active = report.Active || previous.Active
	report.Loaded = report.Loaded || previous.Loaded
	return report, nil
}

func (s Service) clearPending() error {
	path, err := s.pendingPath()
	if err != nil {
		return err
	}
	err = s.Host.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
