package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

type terminalBuffer struct{ bytes.Buffer }

func (*terminalBuffer) Fd() uintptr { return 7 }

func TestPlanColour(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                    string
		preference              string
		terminal, noColor, want bool
	}{
		{name: "terminal", terminal: true, want: true},
		{name: "pipe"},
		{name: "always in pipe", preference: "always", want: true},
		{name: "never on terminal", preference: "never", terminal: true},
		{name: "NO_COLOR beats always", preference: "always", noColor: true},
		{name: "NO_COLOR even empty", terminal: true, noColor: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			values := map[string]string{"HOME": "/home/test"}
			if tc.noColor {
				values["NO_COLOR"] = ""
			}
			options := []fake.Option{fake.WithEnv(values)}
			if tc.terminal {
				options = append(options, fake.WithTerminal(7))
			}
			var output terminalBuffer
			c := commandLine{host: fake.New(options...), stdout: &output}
			c.config.UI.Color = tc.preference
			p := plan.Plan{Operations: []plan.Operation{{Kind: plan.Write, Path: "/output", Classification: plan.UpdateNeeded}}}
			if err := writeDiff(c.stdout, p, c.colour()); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "\x1b[") != tc.want {
				t.Fatalf("colour: %q", output.String())
			}
		})
	}
}
