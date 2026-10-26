//go:build !darwin

package platform

// DetachServiceRefresh is only used by launchd; systemd reloads before exit 75.
func DetachServiceRefresh(string, string, string) error { return ErrUnsupported }
