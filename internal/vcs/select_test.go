package vcs_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestParseKind(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want vcs.Kind
		ok   bool
	}{
		{"auto", vcs.Auto, true},
		{"system", vcs.System, true},
		{"embedded", vcs.Embedded, true},
		{"", "", false},
		{"System", "", false},
		{" system", "", false},
		{"libgit2", "", false},
	} {
		got, err := vcs.ParseKind(tt.in)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("ParseKind(%q) = %q, %v; want %q, ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
		if err != nil && !errors.Is(err, vcs.ErrInvalidKind) {
			t.Errorf("ParseKind(%q) error %v does not match ErrInvalidKind", tt.in, err)
		}
	}
}

func TestSelectPrecedence(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                string
		flag, env, config   string
		wantKind            vcs.Kind
		wantSource          vcs.Source
		wantErr, errMention string
	}{
		{name: "default", wantKind: vcs.Auto, wantSource: vcs.SourceDefault},
		{name: "config", config: "embedded", wantKind: vcs.Embedded, wantSource: vcs.SourceConfig},
		{name: "env beats config", env: "system", config: "embedded", wantKind: vcs.System, wantSource: vcs.SourceEnv},
		{name: "flag beats env", flag: "embedded", env: "system", config: "system", wantKind: vcs.Embedded, wantSource: vcs.SourceFlag},
		{name: "flag auto beats forced env", flag: "auto", env: "system", wantKind: vcs.Auto, wantSource: vcs.SourceFlag},
		{name: "flag beats invalid env", flag: "system", env: "bogus", wantKind: vcs.System, wantSource: vcs.SourceFlag},
		{name: "invalid flag", flag: "bogus", env: "system", errMention: "--git-backend"},
		{name: "invalid env", env: "bogus", config: "system", errMention: vcs.EnvVar},
		{name: "invalid config", config: "libgit2", errMention: "config.toml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := vcs.Select(tt.flag, tt.env, tt.config)
			if tt.errMention != "" {
				if !errors.Is(err, vcs.ErrInvalidKind) || !strings.Contains(err.Error(), tt.errMention) {
					t.Fatalf("Select error = %v, want ErrInvalidKind naming %s", err, tt.errMention)
				}
				return
			}
			if err != nil || got.Kind != tt.wantKind || got.Source != tt.wantSource {
				t.Fatalf("Select = %+v, %v; want %s from %s", got, err, tt.wantKind, tt.wantSource)
			}
			if got.Explicit() != (tt.wantKind != vcs.Auto) {
				t.Errorf("Explicit() = %v for %s", got.Explicit(), got.Kind)
			}
		})
	}
}

// stubBackend is a backend that only reports its identity.
type stubBackend struct {
	vcs.Backend
	kind vcs.Kind
}

func (b stubBackend) Kind() vcs.Kind { return b.kind }
func (stubBackend) Version() string  { return "stub 1.0" }

var errNoGit = vcs.ErrSystemGitNotFound

func stubFactory(kind vcs.Kind, err error) func(context.Context) (vcs.Backend, error) {
	return func(context.Context) (vcs.Backend, error) {
		if err != nil {
			return nil, err
		}
		return stubBackend{kind: kind}, nil
	}
}

func TestOpenForcedNeverFallsBack(t *testing.T) {
	t.Parallel()

	working := vcs.Factories{System: stubFactory(vcs.System, errNoGit), Embedded: stubFactory(vcs.Embedded, nil)}
	sel := vcs.Selection{Kind: vcs.System, Source: vcs.SourceFlag}
	b, res, err := vcs.Open(t.Context(), sel, working)
	if b != nil || res.Active != "" {
		t.Fatalf("forced system without git opened %v (%+v)", b, res)
	}
	var unavailable *vcs.UnavailableError
	if !errors.As(err, &unavailable) || !errors.Is(err, vcs.ErrUnavailable) || !errors.Is(err, errNoGit) {
		t.Fatalf("error = %v, want UnavailableError wrapping the cause", err)
	}
	const want = `Git backend "system" (selected by --git-backend) is unavailable: ` +
		`git was not found in PATH; install Git 2.30 or newer (https://git-scm.com/downloads)`
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
	if res.Kind != vcs.System || res.Source != vcs.SourceFlag {
		t.Errorf("resolution = %+v, want the selection kept for doctor", res)
	}
}

func TestOpenMissingFactory(t *testing.T) {
	t.Parallel()

	for _, sel := range []vcs.Selection{
		{Kind: vcs.Embedded, Source: vcs.SourceEnv},
		{Kind: vcs.Auto, Source: vcs.SourceDefault},
	} {
		// A working system backend must not stand in for a forced embedded one.
		f := vcs.Factories{System: stubFactory(vcs.System, errNoGit)}
		if sel.Kind == vcs.Embedded {
			f.System = stubFactory(vcs.System, nil)
		}
		b, _, err := vcs.Open(t.Context(), sel, f)
		if b != nil || !errors.Is(err, errors.ErrUnsupported) || !errors.Is(err, vcs.ErrUnavailable) {
			t.Errorf("Open(%+v) = %v, %v; want errors.ErrUnsupported", sel, b, err)
		}
		const autoMessage = `Git backend "auto" (selected by default) is unavailable: ` +
			`system Git: git was not found in PATH; embedded: unsupported operation; install Git 2.30 or newer (https://git-scm.com/downloads)`
		if sel.Kind == vcs.Auto && (!errors.Is(err, errNoGit) || err.Error() != autoMessage) {
			t.Errorf("auto error = %q, want %q", err, autoMessage)
		}
	}
}

func TestOpenAuto(t *testing.T) {
	t.Parallel()

	sel := vcs.Selection{Kind: vcs.Auto, Source: vcs.SourceDefault}
	b, res, err := vcs.Open(t.Context(), sel, vcs.Factories{
		System:   stubFactory(vcs.System, nil),
		Embedded: stubFactory(vcs.Embedded, nil),
	})
	if err != nil || b.Kind() != vcs.System || res.Active != vcs.System || res.SystemErr != nil {
		t.Fatalf("auto with git = %v, %+v, %v; want system", b, res, err)
	}
	if got, want := res.String(), "system (stub 1.0), selected by default"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	b, res, err = vcs.Open(t.Context(), sel, vcs.Factories{
		System:   stubFactory(vcs.System, errNoGit),
		Embedded: stubFactory(vcs.Embedded, nil),
	})
	if err != nil || b.Kind() != vcs.Embedded || !errors.Is(res.SystemErr, errNoGit) {
		t.Fatalf("auto without git = %v, %+v, %v; want embedded with the system error", b, res, err)
	}
}

func TestOpenForcedSystem(t *testing.T) {
	t.Parallel()

	sel := vcs.Selection{Kind: vcs.System, Source: vcs.SourceConfig}
	_, res, err := vcs.Open(t.Context(), sel, vcs.Factories{System: stubFactory(vcs.System, nil)})
	if err != nil || res.Active != vcs.System {
		t.Fatalf("Open = %+v, %v", res, err)
	}
	if got, want := res.String(), "system (stub 1.0), selected by git.backend in config.toml"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	_, res, err = vcs.Open(t.Context(), sel, vcs.Factories{})
	if !errors.Is(err, vcs.ErrUnavailable) {
		t.Errorf("Open without factories: error = %v", err)
	}
	if got, want := res.String(), "none (system requested by git.backend in config.toml)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestOpenAutoRejectsBrokenSystem(t *testing.T) {
	t.Parallel()
	broken := errors.New("installed Git is too old or failed to start")
	_, res, err := vcs.Open(t.Context(), vcs.Selection{Kind: vcs.Auto, Source: vcs.SourceDefault}, vcs.Factories{
		System: stubFactory(vcs.System, broken), Embedded: stubFactory(vcs.Embedded, nil),
	})
	if !errors.Is(err, broken) || res.Active != "" {
		t.Fatalf("broken installed Git fell back: %+v, %v", res, err)
	}
}
