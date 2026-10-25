// Package doctor diagnoses synchronization prerequisites through injected contracts.
package doctor

import (
	"context"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Status is the stable diagnostic severity.
type Status string

// Diagnostic severities. Skipped checks are OK with an explicit skipped message.
const (
	OK   Status = "OK"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

// Check is one result. Non-OK results always have a one-line fix.
type Check struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// Report is a versioned, deterministic list in registry order.
type Report struct {
	Checks       []Check `json:"checks"`
	DoctorSchema int     `json:"doctor_schema"`
}

// Service supplies all host, state and process dependencies. Open and VersionProbe
// accept contexts and must respect cancellation. Offline disables remote probes.
type Service struct {
	Host                                    platform.Host
	Store                                   *state.Store
	Open                                    func(context.Context) (vcs.Backend, vcs.Resolution, error)
	VersionProbe                            func(context.Context, string) (string, error)
	Repository, Version, HostFlag, ModeFlag string
	Config                                  config.Config
	Offline                                 bool
}

// Timeout bounds each network/process operation independently.
const Timeout = 2 * time.Second

// Run executes the registry serially and continues after failed prerequisites.
func (s Service) Run(ctx context.Context) Report {
	r := Report{DoctorSchema: 1, Checks: []Check{}}
	for _, check := range []func(context.Context) []Check{s.manifestChecks, s.gitChecks, s.sshChecks, s.agentChecks, s.stateChecks, s.legacyChecks} {
		r.Checks = append(r.Checks, check(ctx)...)
	}
	for i := range r.Checks {
		r.Checks[i].Message = strings.Join(strings.Fields(r.Checks[i].Message), " ")
	}
	return r
}

// Code follows the shared CLI table: minimum-version failure is 4, other FAIL
// results (or WARN under strict) are 1. Ordinary warnings exit successfully.
func (r Report) Code(strict bool) apply.Code {
	code := apply.OK
	for _, check := range r.Checks {
		if check.Status == Fail || strict && check.Status == Warn {
			code = max(code, apply.Warnings)
		}
		if check.ID == "tool-version" && check.Status == Fail {
			code = apply.ToolTooOld
		}
	}
	return code
}

func result(id, message, fix string, err error, severity Status) Check {
	if err == nil {
		return Check{ID: id, Status: OK, Message: message}
	}
	return Check{ID: id, Status: severity, Message: message + ": " + err.Error(), Fix: fix}
}
