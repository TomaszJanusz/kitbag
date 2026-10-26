package platform

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const refreshLogPerm = 0o600

// DetachServiceRefresh starts the complete refresh in a new session. launchd
// reaps the daemon's process group on bootout, so the helper must leave it.
// No inherited stdio or socket/lock descriptors keep the parent alive.
func DetachServiceRefresh(binary, config, logDir string) (err error) {
	log, err := os.OpenFile(filepath.Join(logDir, "service.refresh.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, refreshLogPerm) //nolint:gosec // G304: private platform log directory from the service installation.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, log.Close()) }()
	cmd := exec.Command(binary, "--config", config, "service", "refresh") //nolint:gosec // G204: our resolved executable, separate config argument, no shell.
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the helper if the parent remains alive (for example refresh fails).
	go reapRefresh(cmd)
	return nil
}

func reapRefresh(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		// The helper writes its diagnostic before exiting; Wait only reaps it.
		return
	}
}
