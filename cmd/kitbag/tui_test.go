package main

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

type terminalInput struct{ io.Reader }

func (terminalInput) Fd() uintptr { return 0 }

func TestTUIRefusesWithoutATerminal(t *testing.T) {
	t.Parallel()
	for name, fds := range map[string][]uintptr{"neither": nil, "stdin only": {0}, "stdout only": {7}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := commandLine{host: fake.New(fake.WithTerminal(fds...)), stdout: &terminalBuffer{}}
			err := classified(c.runTUI(t.Context(), terminalInput{strings.NewReader("q")}))
			var failure *commandError
			if !errors.Is(err, fs.ErrInvalid) || !errors.As(err, &failure) || failure.code != apply.Usage {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

// TestTUIRunsAndQuits drives the real renderer through runTUI with a
// terminal-like input that types q.
func TestTUIRunsAndQuits(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}), fake.WithTerminal(0, 7))
	var output terminalBuffer
	c := commandLine{host: host, stdout: &output, stderr: io.Discard}
	if err := c.runTUI(t.Context(), terminalInput{strings.NewReader("q")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[?1049l") {
		t.Fatalf("alternate screen not restored: %q", output.String())
	}
}

func TestTUIOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		env        map[string]string
		preference string
		want       bool
	}{
		{env: map[string]string{}},
		{env: map[string]string{"NO_COLOR": ""}, want: true},
		{env: map[string]string{}, preference: "never", want: true},
		{env: map[string]string{"NO_COLOR": "1"}, preference: "always", want: true},
	} {
		tc.env["HOME"] = "/home/test"
		c := commandLine{host: fake.New(fake.WithEnv(tc.env)), stdout: io.Discard, stderr: io.Discard}
		c.config.UI.Color = tc.preference
		if got := c.tuiOptions(0).NoColour; got != tc.want {
			t.Errorf("env=%v ui.color=%q: NoColour=%v", tc.env, tc.preference, got)
		}
	}
}

// Without a daemon the renderer reads this machine in-process, like status
// and config list; flags still override settings.
func TestTUIFallsBackInProcess(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	c := commandLine{host: host, stdout: io.Discard, stderr: io.Discard, flags: globalFlags{host: "flagged", verbose: true}}
	client := c.tuiOptions(0).Runner.Client
	values, err := client.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range values {
		if value.Key == "host" {
			found = value.Value == "flagged" && value.Source == model.SourceFlag
		}
	}
	if !found {
		t.Fatalf("host override missing: %+v", values)
	}
	snapshot, err := client.Status(t.Context())
	if err == nil || snapshot.Daemon {
		t.Fatalf("status=%+v err=%v", snapshot, err)
	}
}
