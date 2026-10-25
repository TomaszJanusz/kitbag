//go:build darwin || linux

package platform

import (
	"context"
	"io"
	"os/exec"
)

// RunEditor launches the user's explicit EDITOR shell command with the draft as
// a separate positional argument. Streams are supplied by the command renderer.
func RunEditor(ctx context.Context, editor, path string, input io.Reader, output, diagnostics io.Writer) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", editor+` "$@"`, "kitbag-editor", path) //nolint:gosec // G204: EDITOR is an explicitly selected user command; the filename is a separate argument.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, output, diagnostics
	return cmd.Run()
}
