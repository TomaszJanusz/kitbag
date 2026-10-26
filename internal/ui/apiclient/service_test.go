package apiclient

import (
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
