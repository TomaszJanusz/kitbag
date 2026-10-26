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
