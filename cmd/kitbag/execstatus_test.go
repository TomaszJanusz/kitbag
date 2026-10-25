package main

import (
	"errors"
	"os/exec"
	"strconv"

	"github.com/rogpeppe/go-internal/testscript"
)

// execStatus runs a real registered binary and asserts its exact exit code while
// retaining normal testscript stdout/stderr assertions.
func execStatus(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: execstatus code command [args...]")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	err = ts.Exec(args[1], args[2:]...)
	got := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		got = exit.ExitCode()
	} else if err != nil {
		ts.Fatalf("execstatus: %v", err)
	}
	if got != want {
		ts.Fatalf("exit status = %d; want %d", got, want)
	}
}
