package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func TestCLIReference(t *testing.T) {
	t.Parallel()
	c := &commandLine{host: noGit(), stdout: io.Discard, stderr: io.Discard}
	root := c.root()
	update, _ := newHost().LookupEnv("KITBAG_UPDATE_CLI")
	directory := "../../docs/cli"
	if update == "1" {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	commands := append([]*cobra.Command{root}, root.Commands()...)
	for _, cmd := range commands {
		checkReference(t, cmd, directory, update)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(commands) {
		t.Errorf("unexpected CLI reference files: got %d, want %d", len(entries), len(commands))
	}
}

func referenceSuffix(cmd *cobra.Command) string {
	if cmd.HasParent() {
		return "_" + cmd.Name()
	}
	return ""
}

func checkReference(t *testing.T, cmd *cobra.Command, directory, update string) {
	t.Helper()
	var out bytes.Buffer
	cmd.DisableAutoGenTag = true
	if err := doc.GenMarkdown(cmd, &out); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "kitbag"+referenceSuffix(cmd)+".md")
	if update == "1" {
		if err := os.WriteFile(filename, out.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filename) //nolint:gosec // G304: filename is derived only from the fixed Cobra tree and docs/cli directory.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, out.Bytes()) {
		t.Errorf("%s is stale; run mise run docs:cli", filename)
	}
}
