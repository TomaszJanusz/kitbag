// Command kitbag manages coding-agent configuration across machines.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// version is the release version, set at build time with
// -ldflags "-X main.version=<semver>"; development builds report "dev".
var version = "dev"

const (
	usage             = "usage: kitbag [flags] <config|daemon|init|sync|plan|status|doctor|adopt|unmanage|revert|self-update|version>"
	exitFailure       = int(apply.Warnings)
	exitUsage         = int(apply.Usage)
	exitUnsupportedOS = int(apply.UnsupportedOS)
)

func main() { os.Exit(run(runtime.GOOS, newHost(), os.Args[1:], os.Stdout, os.Stderr)) }

func run(goos string, host platform.Host, args []string, stdout, stderr io.Writer) int {
	if err := platform.CheckOS(goos); err != nil {
		return report(stderr, exitUnsupportedOS, err)
	}
	cli := &commandLine{host: host, stdout: stdout, stderr: stderr, process: true}
	root := cli.root()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var failure *commandError
		if errors.As(err, &failure) {
			return report(stderr, int(max(cli.code, failure.code)), failure.err)
		}
		return report(stderr, exitUsage, err)
	}
	return int(cli.code)
}

func report(stderr io.Writer, code int, args ...any) int {
	if _, err := fmt.Fprintln(stderr, append([]any{"kitbag:"}, args...)...); err != nil {
		return exitFailure
	}
	return code
}

type commandError struct {
	err  error
	code apply.Code
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }
func classified(err error) error {
	if err == nil {
		return nil
	}
	var remote interface{ ExitCode() apply.Code }
	if errors.As(err, &remote) {
		return &commandError{err: err, code: remote.ExitCode()}
	}
	return &commandError{err: err, code: apply.ErrorCode(err)}
}
