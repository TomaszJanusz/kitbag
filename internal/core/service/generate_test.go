package service

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
)

func TestGoldenDefinitions(t *testing.T) {
	t.Parallel()
	for _, manager := range []string{"systemd", "launchd"} {
		for _, mode := range []string{"daemon", "timer"} {
			t.Run(manager+"/"+mode, func(t *testing.T) {
				t.Parallel()
				options := Options{Binary: `/opt/Kit Bag/bin/kitbag`, Config: `/home/test/Config & "files"/config.toml`, LogDir: "/home/test/log", Mode: mode, Interval: DefaultInterval, Environment: map[string]string{"HOME": "/home/test", "PATH": "/opt/Kit Bag/bin:/usr/bin:/bin"}}
				definitions, err := Generate(manager, options)
				must(t, err)
				for name, text := range definitions {
					want, readErr := fs.ReadFile(goldens(), "testdata/"+manager+"-"+mode+"-"+name+".golden")
					must(t, readErr)
					if text != string(want) {
						t.Errorf("%s mismatch\ngot:\n%s\nwant:\n%s", name, text, want)
					}
				}
			})
		}
	}
}

// goldens keeps the embedded fixture immutable and avoids package globals.
func goldens() fs.FS { return goldenFiles }

//go:embed testdata/*.golden
var goldenFiles embed.FS

func TestSystemdEscaping(t *testing.T) {
	t.Parallel()
	_, _, options := subject(t)
	options.Binary = `/opt/a%h$HOME/kitbag`
	options.Environment["PATH"] = `/opt/$literal:%h`
	definitions, err := Generate("systemd", options)
	must(t, err)
	text := definitions["dev.kitbag.agent.service"]
	if !strings.Contains(text, `"/opt/a%%h$$HOME/kitbag"`) || !strings.Contains(text, `Environment="PATH=/opt/$literal:%%h"`) {
		t.Fatal(text)
	}
}

// TestDaemonDefinitionsRestartOnRestartExitCode guards the contract with the
// daemon: after its executable is replaced it exits with RestartExitCode, and
// the supervisor must start it again. systemd's Restart=on-failure (or always)
// restarts after a nonzero status unless SuccessExitStatus or
// RestartPreventExitStatus lists it; launchd's unconditional KeepAlive restarts
// after any exit, while a KeepAlive dictionary (SuccessfulExit) might not.
func TestDaemonDefinitionsRestartOnRestartExitCode(t *testing.T) {
	t.Parallel()
	if RestartExitCode == 0 {
		t.Fatal("a zero restart status is a clean exit that on-failure does not restart")
	}
	_, _, options := subject(t)
	options.Mode = "daemon"
	definitions, err := Generate("systemd", options)
	must(t, err)
	unit := definitions["dev.kitbag.agent.service"]
	restart := ""
	for line := range strings.SplitSeq(unit, "\n") {
		key, value, _ := strings.Cut(line, "=")
		switch key {
		case "Restart":
			restart = value
		case "SuccessExitStatus", "RestartPreventExitStatus":
			t.Fatalf("%s may exclude the restart status: %s", key, line)
		}
	}
	if restart != "on-failure" && restart != "always" {
		t.Fatalf("Restart=%q does not restart after status %d:\n%s", restart, RestartExitCode, unit)
	}
	definitions, err = Generate("launchd", options)
	must(t, err)
	if plist := definitions["dev.kitbag.agent.plist"]; !strings.Contains(plist, "<key>KeepAlive</key><true/>") {
		t.Fatalf("KeepAlive is not unconditional:\n%s", plist)
	}
}
