//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

func TestDaemonRefreshProcessHelper(t *testing.T) {
	t.Parallel()
	if !slices.Contains(os.Args, "--refresh-probe") {
		return
	}
	os.Exit(run(runtime.GOOS, newHost(), []string{"daemon"}, os.Stdout, os.Stderr))
}

func TestDaemonStartupRefreshSubprocess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("systemd exit path; launchd helper is tested separately")
	}
	f := newDaemonFixture(t)
	stub := filepath.Join(f.home, "bin", "systemctl")
	f.writeExecutable(t, stub, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/supervisor-calls\"\ncase \"$2\" in show) printf 'LoadState=loaded\\nActiveState=active\\nUnitFileState=enabled\\n';; esac\n")
	for i, entry := range f.env {
		if strings.HasPrefix(entry, "PATH=") {
			f.env[i] = "PATH=" + filepath.Dir(stub) + ":" + strings.TrimPrefix(entry, "PATH=")
		}
	}
	host := unix.New(unix.WithEnv(f.lookup), unix.WithServiceRunner(func(_ context.Context, _ string, _, _ []string) (string, int, error) {
		return "LoadState=loaded\nActiveState=active\nUnitFileState=enabled", 0, nil
	}))
	s := service.Service{Host: host}
	options, err := s.Current("daemon", service.DefaultInterval)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Install(t.Context(), options, false); err != nil {
		t.Fatal(err)
	}
	layout, err := host.ServiceLayout()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(layout.Directory, "dev.kitbag.agent.service")
	f.write(t, path, "[Service]\nRestart=no\nExecStart=/old/kitbag daemon\n")
	command := daemonProcess(t, f)
	command.Args = []string{command.Path, "-test.run=^TestDaemonRefreshProcessHelper$", "--", "--refresh-probe"}
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != service.RestartExitCode {
		t.Fatalf("exit=%v output=%s", err, output)
	}
	assertStartupDefinition(t, host, path, f.home)
	assertDaemonReleased(t, f)
	// The new definition is current; another daemon stays alive and serves.
	second, secondOutput := startDaemonProcess(t, f)
	waitDaemonFile(t, f, "first")
	stopDaemonProcess(t, f, second, secondOutput)
}

func assertStartupDefinition(t *testing.T, host platform.Host, path, home string) {
	t.Helper()
	data, err := host.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "Restart=on-failure") {
		t.Fatalf("definition=%s err=%v", data, err)
	}
	data, err = host.ReadFile(filepath.Join(home, "supervisor-calls"))
	if err != nil || !strings.Contains(string(data), "--user daemon-reload") || strings.Contains(string(data), " restart ") {
		t.Fatalf("calls=%s err=%v", data, err)
	}
}
