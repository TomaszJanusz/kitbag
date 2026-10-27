package client

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestVersionObservationOldAndNegativeControls(t *testing.T) {
	t.Parallel()
	for _, err := range []error{nil, io.EOF, net.ErrClosed, context.DeadlineExceeded, &api.Error{Code: api.MethodNotFound}} {
		version, running, failure := VersionObservation(api.VersionInfo{}, err)
		if failure != nil || version != "older" || !running {
			t.Fatal(version, running, failure)
		}
	}
}

func TestVersionObservationNegativeControls(t *testing.T) {
	t.Parallel()
	version, running, err := VersionObservation(api.VersionInfo{ToolVersion: "0.1.2"}, nil)
	if err != nil || !running || version != "0.1.2" {
		t.Fatal(version, running, err)
	}
	version, running, err = VersionObservation(api.VersionInfo{}, platform.ErrNoDaemon)
	if err != nil || running || version != "" {
		t.Fatal(version, running, err)
	}
	denied := &api.Error{Code: api.Unauthorized}
	_, running, err = VersionObservation(api.VersionInfo{}, denied)
	if !errors.Is(err, denied) || running {
		t.Fatal(running, err)
	}
}

func TestRecoveryVersionObservationRejectsTransportFailures(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{io.EOF, net.ErrClosed, context.DeadlineExceeded, &api.Error{Code: api.Unauthorized}, &api.Error{Code: api.InvalidParams}} {
		version, running, err := RecoveryVersionObservation(api.VersionInfo{ToolVersion: "0.1.2"}, failure)
		if !errors.Is(err, failure) || running || version != "" {
			t.Fatal("failed observation authorized recovery", version, running, err)
		}
	}
}

func TestRecoveryVersionObservationAcceptsReliableLegacyReplies(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{nil, &api.Error{Code: api.MethodNotFound}} {
		version, running, err := RecoveryVersionObservation(api.VersionInfo{}, failure)
		if err != nil || !running || version != "older" {
			t.Fatal(version, running, err)
		}
	}
	version, running, err := RecoveryVersionObservation(api.VersionInfo{ToolVersion: "0.1.2"}, nil)
	if err != nil || !running || version != "0.1.2" {
		t.Fatal(version, running, err)
	}
}

func TestRecoveryVersionObservationNoDaemon(t *testing.T) {
	t.Parallel()
	version, running, err := RecoveryVersionObservation(api.VersionInfo{}, platform.ErrNoDaemon)
	if err != nil || running || version != "" {
		t.Fatal(version, running, err)
	}
}
