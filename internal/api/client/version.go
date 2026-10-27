package client

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// DaemonVersion also works with pre-self-heal API v1 daemons. No daemon is a
// normal observation; authentication/protocol errors remain errors.
func (c Client) DaemonVersion(ctx context.Context) (version string, running bool, err error) {
	var info api.VersionInfo
	err = c.Call(ctx, "version.get", api.Params{}, &info, nil)
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	return VersionObservation(info, err)
}

// VersionObservation recognizes old v1 daemons with absent version metadata or
// no version method. Authentication and incompatible-protocol errors still fail.
func VersionObservation(info api.VersionInfo, err error) (string, bool, error) {
	if errors.Is(err, platform.ErrNoDaemon) {
		return "", false, nil
	}
	if versionHandshakeMissing(err) {
		return "older", true, nil
	}
	if err == nil && info.ToolVersion == "" {
		return "older", true, nil
	}
	return info.ToolVersion, err == nil, err
}

func versionHandshakeMissing(err error) bool {
	var rpc *api.Error
	return errors.As(err, &rpc) && rpc.Code == api.MethodNotFound || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.DeadlineExceeded)
}

// RecoveryDaemonVersion observes versions for automatic recovery. Transport
// failures never imply that an active daemon is an older version.
func (c Client) RecoveryDaemonVersion(ctx context.Context) (string, bool, error) {
	var info api.VersionInfo
	err := c.Call(ctx, "version.get", api.Params{}, &info, nil)
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	return RecoveryVersionObservation(info, err)
}

// RecoveryVersionObservation accepts a successful reply or an authenticated
// method-not-found response from an old API. Explicit repair retains the looser
// legacy VersionObservation behavior for old daemons that close the handshake.
func RecoveryVersionObservation(info api.VersionInfo, err error) (string, bool, error) {
	var rpc *api.Error
	if err != nil && !errors.Is(err, platform.ErrNoDaemon) && (!errors.As(err, &rpc) || rpc.Code != api.MethodNotFound) {
		return "", false, err
	}
	return VersionObservation(info, err)
}
