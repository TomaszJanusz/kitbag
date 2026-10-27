package apiclient

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestOldDaemonVersionAndLocalRefresh(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	s := service.Service{Host: host}
	options, err := s.Current("daemon", service.DefaultInterval)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Install(t.Context(), options, false); err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{results: map[string]any{"status.get": workflow.Status{StatusSchema: 1}, "version.get": api.VersionInfo{ToolVersion: "0.1.1"}}}
	c := Client{Host: host, Transport: transport, BinaryVersion: "0.1.3"}
	snapshot, err := c.Status(t.Context())
	if err != nil || snapshot.DaemonVersion != "0.1.1" || snapshot.BinaryVersion != "0.1.3" || !snapshot.ServiceInstalled {
		t.Fatal(snapshot, err)
	}
	before := len(host.ServiceCalls())
	if err = c.RefreshService(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := host.ServiceCalls()[before:]
	if len(calls) != 2 || calls[1] != "restart timer=false" {
		t.Fatal(calls)
	}
	assertRefreshCalls(t, transport)
}

func assertRefreshCalls(t *testing.T, transport *fakeTransport) {
	t.Helper()
	for _, call := range transport.calls {
		if call.method != "status.get" && call.method != "version.get" {
			t.Fatal("refresh used daemon RPC", call)
		}
	}
}

func TestStatusRecoversOldDaemonAutomaticallyOnce(t *testing.T) {
	t.Parallel()
	c, transport, host := recoveryClient(t)
	before := len(host.ServiceCalls())
	snapshot, err := c.Status(t.Context())
	if err != nil || !snapshot.ServiceRecovering || len(host.ServiceCalls()) == before {
		t.Fatal(snapshot, err, host.ServiceCalls())
	}
	checkRecoveryRepeat(t, c, transport, host)
}

func checkRecoveryRepeat(t *testing.T, c Client, transport *fakeTransport, host *fake.Host) {
	t.Helper()
	before := len(host.ServiceCalls())
	snapshot, err := c.Status(t.Context())
	if err != nil || !snapshot.ServiceRecovering || len(host.ServiceCalls()) != before {
		t.Fatal("repeated recovery", snapshot, err, host.ServiceCalls())
	}
	transport.results["version.get"] = api.VersionInfo{ToolVersion: c.BinaryVersion}
	snapshot, err = c.Status(t.Context())
	if err != nil || snapshot.ServiceRecovering || len(host.ServiceCalls()) != before {
		t.Fatal("completed recovery", snapshot, err, host.ServiceCalls())
	}
	assertRefreshCalls(t, transport)
}

func recoveryClient(t *testing.T) (Client, *fakeTransport, *fake.Host) {
	t.Helper()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	s := service.Service{Host: host}
	options, err := s.Current("daemon", service.DefaultInterval)
	if err != nil {
		t.Fatal(err)
	}
	if err = host.MkdirAll(filepath.Dir(options.Binary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = host.WriteAtomic(options.Binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Install(t.Context(), options, false); err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{results: map[string]any{"status.get": workflow.Status{StatusSchema: 1}, "version.get": api.VersionInfo{ToolVersion: "0.1.2"}}}
	c := Client{Host: host, Transport: transport, BinaryVersion: "0.1.3"}
	return c, transport, host
}

func TestStatusDoesNotRecoverAfterUnauthorizedVersionObservation(t *testing.T) {
	t.Parallel()
	c, transport, host := recoveryClient(t)
	transport.errs = map[string]error{"version.get": &api.Error{Code: api.Unauthorized, Message: "unauthorized"}}
	before := len(host.ServiceCalls())
	snapshot, err := c.Status(t.Context())
	if err != nil || snapshot.ServiceRecovering || len(host.ServiceCalls()) != before {
		t.Fatal("unauthorized observation restarted service", snapshot, err, host.ServiceCalls())
	}
}
