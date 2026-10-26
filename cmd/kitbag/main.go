// Command kitbag manages coding-agent configuration across machines.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/timefmt"
)

// version is the release version, set at build time with
// -ldflags "-X main.version=<semver>"; development builds report "dev".
var version = "dev"

const (
	usage             = "usage: kitbag [flags] <agent|config|service|daemon|init|sync|plan|status|tui|doctor|adopt|unmanage|revert|self-update|version>"
	firstRun          = "first time here? run kitbag init to create or connect your configuration repository"
	exitFailure       = int(apply.Warnings)
	exitUsage         = int(apply.Usage)
	exitUnsupportedOS = int(apply.UnsupportedOS)
)

func main() { os.Exit(run(runtime.GOOS, newHost(), os.Args[1:], os.Stdout, os.Stderr)) }

func run(goos string, host platform.Host, args []string, stdout, stderr io.Writer) int {
	if err := platform.CheckOS(goos); err != nil {
		return report(stderr, exitUnsupportedOS, err)
	}
	cli := &commandLine{host: host, stdout: stdout, stderr: stderr, process: true, clock: timefmt.Clock{Location: time.Local, Now: time.Now}}
	return cli.execute(context.Background(), args)
}

// execute runs one command line and maps its outcome to the exit status.
func (c *commandLine) execute(ctx context.Context, args []string) int {
	root := c.root()
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		var failure *commandError
		if errors.As(err, &failure) {
			return report(c.stderr, int(max(c.code, failure.code)), failure.err)
		}
		return report(c.stderr, exitUsage, err)
	}
	return int(c.code)
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
