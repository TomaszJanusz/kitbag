package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/update"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/releasehttp"
	"github.com/TomaszJanusz/kitbag/internal/releasesig"
)

const releaseBaseURL = "https://github.com/TomaszJanusz/kitbag/releases"

// KIT-39 must provision the maintainer's production public key before releasing.
// Empty trust fails closed. Never use a generated fixture key for production.
const releasePublicKey = ""

func (c *commandLine) selfUpdateCommand() *cobra.Command {
	return c.updateCommand(c.selfUpdateService)
}

func (c *commandLine) updateCommand(factory func() (update.Service, error)) *cobra.Command {
	var channel string
	var check, dryRun bool
	cmd := &cobra.Command{
		Use: "self-update", Short: "Verify and install a newer release or show the package manager command",
		Long: "Respect the install-source stamp. Direct installs verify a cosign-signed checksum file and archive before atomic replacement and version confirmation. Unknown sources print reinstall guidance. --check and --dry-run verify without writing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := factory()
			if err != nil {
				return classified(err)
			}
			result, err := service.Run(cmd.Context(), update.Options{Channel: channel, Check: check || dryRun})
			if err != nil {
				return classified(err)
			}
			_, err = fmt.Fprintf(c.stdout, "%s\n", updateMessage(result, check || dryRun))
			return classified(err)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "Release channel: stable or nightly")
	cmd.Flags().BoolVar(&check, "check", false, "Verify the available release without installing")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify the available release without installing (alias for --check)")
	return cmd
}

func updateMessage(result update.Result, check bool) string {
	if check && result.Version != "" {
		return "verified kitbag " + result.Version + " (" + result.Source + ")"
	}
	return result.Command
}

func (c *commandLine) selfUpdateService() (update.Service, error) {
	binary, err := platform.Executable()
	if err != nil {
		return update.Service{}, err
	}
	service := update.Service{
		FS: c.host, Binary: binary, Current: toolVersion(), OS: runtime.GOOS, Arch: runtime.GOARCH,
		BaseURL: releaseBaseURL, Fetch: releasehttp.New().Fetch,
		Probe: platform.ProbeExecutable, Exec: platform.Reexec,
	}
	if base, ok := c.host.LookupEnv("KITBAG_RELEASE_BASE_URL"); ok {
		service.BaseURL = base
	}
	service.Verify = func(data, sig []byte) error {
		verify, keyErr := releasesig.Verifier([]byte(releasePublicKey))
		if keyErr != nil {
			return keyErr
		}
		return verify(data, sig)
	}

	return service, nil
}
