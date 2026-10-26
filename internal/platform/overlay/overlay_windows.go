// Package overlay replaces selected environment variables of a host. Windows
// refuses the daemon and agent commands, so the overlay is the host itself.
package overlay

import "github.com/TomaszJanusz/kitbag/internal/platform"

// Environment returns host unchanged on Windows.
func Environment(host platform.Host, _ []string, _ map[string]string) platform.Host {
	return host
}
