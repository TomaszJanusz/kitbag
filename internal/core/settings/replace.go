package settings

import "github.com/TomaszJanusz/kitbag/internal/config"

// Replace validates complete TOML before atomically replacing machine settings.
// It shares the CLI settings lock and regular-file checks. Running operations
// retain their configuration; subsequent operations reload the replacement.
func (s Service) Replace(data []byte) error {
	if _, err := config.Parse(data); err != nil {
		return err
	}
	return s.locked(func() error { return s.Host.WriteAtomic(s.Filename, data, filePerm) })
}
