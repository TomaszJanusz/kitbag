package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/initialize"
	"github.com/TomaszJanusz/kitbag/internal/core/settings"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (c *commandLine) settingsService() (settings.Service, error) {
	filename, err := initialize.ConfigPath(c.host, c.flags.config)
	if err != nil {
		return settings.Service{}, err
	}
	filename, err = filepath.Abs(filename)
	return settings.Service{Host: c.host, Filename: filename, Flags: map[string]string{"host": c.flags.host, "mode": c.flags.mode, "git-backend": c.flags.gitBackend}}, err
}

func (c *commandLine) configCommand() *cobra.Command {
	parent := &cobra.Command{Use: "config", Short: "Inspect and change machine settings", Long: "Manage config.toml with validated atomic writes. Precedence: flag > KITBAG_* environment > config.toml > default. Use edit for comment-bearing files and paired identity changes."}
	for _, spec := range []struct {
		use, short string
		count      int
	}{
		{"get <key>", "Print an effective value", 1},
		{"set <key> <value>", "Validate and persist a setting", 2},
		{"unset <key>", "Remove a stored optional setting", 1},
		{"list", "List effective values and their sources", 0},
		{"explain <key>", "Describe a setting, default, effective value and source", 1},
		{"edit", "Edit a private draft with $EDITOR and validate before saving", 0},
	} {
		parent.AddCommand(&cobra.Command{Use: spec.use, Short: spec.short, Args: cobra.ExactArgs(spec.count), RunE: func(cmd *cobra.Command, args []string) error {
			service, err := c.settingsService()
			if err != nil {
				return classified(err)
			}
			return classified(c.runConfig(cmd, service, args))
		}})
	}
	return parent
}

func (c *commandLine) runConfig(cmd *cobra.Command, s settings.Service, args []string) error {
	switch cmd.Name() {
	case "set", "unset":
		value := ""
		if cmd.Name() == "set" {
			value = args[1]
		}
		unused, err := s.ChangeWorkingCopy(args[0], value, cmd.Name() == "unset")
		if err != nil || unused == "" {
			return err
		}
		_, err = fmt.Fprintf(c.stderr, "note: working copy %s is no longer used; kitbag never deletes it. Remove it yourself once it holds nothing you need (check git status and unpushed commits).\n", unused)
		return err
	case "edit":
		return c.editConfig(cmd, s)
	case "list":
		return c.listConfig(s)
	case "get", "explain":
		entry, err := s.Get(args[0])
		if err != nil {
			return err
		}
		if cmd.Name() == "get" {
			_, err = fmt.Fprintln(c.stdout, entry.Value)
		} else {
			_, err = fmt.Fprintf(c.stdout, "%s: %s\nAllowed: %s\nDefault: %s\nCurrent: %s\nSource: %s\n", entry.Key.Path, entry.Key.Description, entry.Key.Constraint(), entry.Key.DefaultDescription(), entry.Display(), entry.Source)
		}
		return err
	}
	return nil
}

func (c *commandLine) editConfig(cmd *cobra.Command, s settings.Service) error {
	editor, _ := c.host.LookupEnv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		return fmt.Errorf("set EDITOR before config edit: %w", fs.ErrInvalid)
	}
	input := cmd.InOrStdin()
	scanner := bufio.NewScanner(input)
	return s.Edit(func(path string) error {
		return platform.RunEditor(cmd.Context(), editor, path, input, c.stdout, c.stderr)
	}, func(validation error) (bool, error) {
		if _, err := fmt.Fprintf(c.stderr, "%v\nRe-edit? [y/N] ", validation); err != nil {
			return false, err
		}
		if !scanner.Scan() {
			return false, scanner.Err()
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes", nil
	})
}

func (c *commandLine) listConfig(s settings.Service) error {
	entries, err := s.List()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err = fmt.Fprintf(c.stdout, "%s = %s (%s)\n", entry.Key.Path, entry.Display(), entry.Source); err != nil {
			return err
		}
	}
	return nil
}
