package main

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
)

// serviceFlags are init's and adopt's --service/--no-service choice.
type serviceFlags struct{ install, decline bool }

func (f *serviceFlags) register(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.install, "service", false, "Install and start the background service (daemon mode) without asking")
	cmd.Flags().BoolVar(&f.decline, "no-service", false, "Do not install the background service and do not ask")
	cmd.MarkFlagsMutuallyExclusive("service", "no-service")
}

func (f serviceFlags) chosen() bool { return f.install || f.decline }

// offerService installs the background service as part of setup: --service
// installs, --no-service declines, a terminal asks with yes as the default,
// and any other run prints how to install it. It never installs silently.
// configFile is the configuration the service will use; empty selects the
// default. Messages go to out, so JSON output can keep stdout clean. An
// existing installation is left alone unless --service asks to update it.
func (c *commandLine) offerService(ctx context.Context, out io.Writer, configFile string, flags serviceFlags, interactive bool) error {
	supervisor := service.Service{Host: c.host, Config: configFile}
	if installed, err := supervisor.Installed(); err == nil && installed && !flags.install {
		return nil
	}
	later := "kitbag service install"
	if configFile != "" {
		later = "kitbag --config " + shellQuote(configFile) + " service install"
	}
	install, err := c.serviceWanted(ctx, out, service.Decide(flags.install, flags.decline, interactive), later)
	if err != nil || !install {
		return err
	}
	installation, err := supervisor.InstallDefault(ctx)
	if err != nil {
		return fmt.Errorf("background service installation failed; fix the cause, then run %s: %w", later, err)
	}
	return c.printInstallation(ctx, out, installation)
}

// serviceWanted resolves the offer, asking on a terminal, and otherwise
// explains how to install the service later.
func (c *commandLine) serviceWanted(ctx context.Context, out io.Writer, offer service.Offer, later string) (bool, error) {
	reason := ""
	switch offer {
	case service.OfferInstall:
		return true, nil
	case service.OfferDecline:
		reason = " (--no-service)"
	case service.OfferHint:
		reason = ": this machine syncs only when you run kitbag sync. Pass --service to install it, or"
	case service.OfferAsk:
		var accepted bool
		if err := serviceForm(&accepted).WithOutput(c.stdout).RunWithContext(ctx); err != nil || accepted {
			return accepted, err
		}
	}
	if offer == service.OfferHint {
		_, err := fmt.Fprintf(out, "Background service not installed%s run later: %s\n", reason, later)
		return false, err
	}
	_, err := fmt.Fprintf(out, "Background service not installed%s. Install it later with: %s\n", reason, later)
	return false, err
}

// serviceForm asks once; the default answer is yes.
func serviceForm(accepted *bool) *huh.Form {
	*accepted = true
	return huh.NewForm(huh.NewGroup(huh.NewConfirm().
		Title("Install the background service now? (recommended)").
		Description("A per-user daemon starts now: its first sync applies the changes shown above, then it keeps this machine in sync. No administrator rights are needed; remove it with kitbag service uninstall.").
		Value(accepted)))
}
