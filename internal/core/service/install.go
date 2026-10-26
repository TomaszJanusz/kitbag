package service

import "context"

// InstallObserved also reconciles the process when the definition is identical.
// A package manager's stable path can hide an old running daemon. Unknown
// versions on an active daemon are older; noStart never restarts that process.
func (s Service) InstallObserved(ctx context.Context, options Options, noStart bool, request RefreshRequest) (Installation, error) {
	before, err := s.Host.ServiceInspect(ctx, options.Mode == "timer")
	if err != nil {
		return Installation{}, err
	}
	request = runningVersion(Report{Mode: options.Mode, ServiceStatus: before}, request)
	changed, err := s.Install(ctx, options, noStart)
	r := installationState(options.Mode, before.Active, noStart, changed, request)
	if err != nil {
		return r, err
	}
	if !changed && before.Active && !noStart {
		refreshed, refreshErr := s.Refresh(ctx, request)
		if refreshErr != nil {
			return r, refreshErr
		}
		r.Changed = refreshed.DefinitionChanged
		r.Restarted = r.Restarted && refreshed.RestartNeeded
	}
	r.Paths, err = s.Paths(options)
	return r, err
}

func installationState(mode string, active, noStart, changed bool, request RefreshRequest) Installation {
	r := Installation{Mode: mode, Changed: changed, DaemonVersion: request.DaemonVersion, BinaryVersion: request.BinaryVersion}
	mismatch := mode == "daemon" && active && request.DaemonVersion != request.BinaryVersion
	r.Started = mode == "daemon" && !noStart && !active
	r.Deferred = noStart && mismatch
	r.Restarted = mismatch && !noStart
	return r
}

func runningVersion(report Report, request RefreshRequest) RefreshRequest {
	if report.Mode == "daemon" && report.Active && request.BinaryVersion != "" {
		request.Running = true
		if request.DaemonVersion == "" {
			request.DaemonVersion = "older"
		}
	}
	return request
}
