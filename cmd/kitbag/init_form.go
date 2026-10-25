package main

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/TomaszJanusz/kitbag/internal/core/initialize"
)

func initForm(flags *initFlags, backendSet, modeSet bool) *huh.Form {
	fields := []huh.Field{}
	if flags.repo == "" {
		fields = append(fields, huh.NewInput().Title("Configuration repository URL").Value(&flags.repo).Validate(requiredValue))
	}
	if !backendSet {
		fields = append(fields, huh.NewSelect[string]().Title("Git backend").Options(huh.NewOption("Automatic", "auto"), huh.NewOption("System Git", "system"), huh.NewOption("Embedded (no Git executable needed)", "embedded")).Value(&flags.backend))
	}
	if !modeSet {
		fields = append(fields, huh.NewSelect[string]().Title("Materialization mode").Options(huh.NewOption("Link", "link"), huh.NewOption("Copy", "copy")).Value(&flags.mode))
	}
	if len(fields) == 0 {
		fields = append(fields, huh.NewNote().Title("Repository choices supplied by flags").Next(true))
	}
	return huh.NewForm(huh.NewGroup(fields...).Description(flags.detected))
}

func identityForm(flags *initFlags) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Fallback embedded commit name (Git identity is missing)").Value(&flags.name).Validate(requiredValue),
		huh.NewInput().Title("Fallback embedded commit email").Value(&flags.email).Validate(func(value string) error {
			if !strings.Contains(value, "@") {
				return fmt.Errorf("enter an email address: %w", fs.ErrInvalid)
			}
			return nil
		}),
	))
}

func requiredValue(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("a value is required: %w", fs.ErrInvalid)
	}
	return nil
}

func confirmationForm(confirmed *bool, title string) *huh.Form {
	return huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(title).Value(confirmed)))
}

func (c *commandLine) initConfirm(ctx context.Context, yes bool, title string) (bool, error) {
	if yes {
		return true, nil
	}
	if !c.host.IsTerminal(0) {
		return false, fmt.Errorf("interactive init needs a terminal; supply --yes: %w", fs.ErrInvalid)
	}
	confirmed := false
	err := confirmationForm(&confirmed, title).WithOutput(c.stdout).RunWithContext(ctx)
	return confirmed, err
}

func detectionDescription(detected []initialize.Agent) string {
	lines := []string{"Existing agent directories (targets and instances are controlled by kitbag.toml):"}
	for _, agent := range detected {
		lines = append(lines, agent.Kind+": "+agent.Directory)
	}
	if len(detected) == 0 {
		lines = append(lines, "none")
	}
	return strings.Join(lines, "\n")
}

func scaffoldForm(flags *initFlags, agentsSet, instancesSet bool) *huh.Form {
	fields := []huh.Field{}
	if !agentsSet {
		fields = append(fields, huh.NewMultiSelect[string]().Title("Agents for the new manifest").Options(huh.NewOption("Claude Code", "claude"), huh.NewOption("Codex", "codex")).Value(&flags.agents))
	}
	if !instancesSet {
		fields = append(fields, huh.NewInput().Title("Instances: agent.name=~/directory, comma separated (blank uses agent defaults)").Value(&flags.instanceText))
	}
	if len(fields) == 0 {
		fields = append(fields, huh.NewNote().Title("Scaffold choices supplied by flags").Next(true))
	}
	return huh.NewForm(huh.NewGroup(fields...).Description(flags.detected))
}
