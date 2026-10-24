package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		code int
		out  string
		err  string
	}{
		{name: "version", args: []string{"version"}, out: "kitbag dev\n"},
		{name: "missing command", code: 2, err: "usage: kitbag version\n"},
		{name: "unknown command", args: []string{"sync"}, code: 2, err: "usage: kitbag version\n"},
		{name: "extra argument", args: []string{"version", "extra"}, code: 2, err: "usage: kitbag version\n"},
		{name: "flag is not a command", args: []string{"--version"}, code: 2, err: "usage: kitbag version\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if got := stdout.String(); got != tt.out {
				t.Errorf("stdout = %q, want %q", got, tt.out)
			}
			if got := stderr.String(); got != tt.err {
				t.Errorf("stderr = %q, want %q", got, tt.err)
			}
		})
	}
}

func TestRunOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"version"}, failedWriter{}, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "output unavailable") {
		t.Errorf("stderr = %q, want output failure", stderr.String())
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}
