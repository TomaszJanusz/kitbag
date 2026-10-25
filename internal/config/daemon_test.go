package config

import (
	"errors"
	"io/fs"
	"testing"
)

func TestDaemonDurationBounds(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{
		"[sync]\npoll_interval='0s'", "[sync]\npoll_interval='999ms'", "[sync]\npoll_interval='25h'", "[sync]\npoll_interval='999999999999999999999h'",
		"[daemon]\ndebounce='0ms'", "[daemon]\ndebounce='99ms'", "[daemon]\ndebounce='61s'",
	} {
		_, err := Parse([]byte("schema=1\n[repo]\nurl='local.git'\n" + setting))
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("accepted %s: %v", setting, err)
		}
	}
	for _, setting := range []string{"[sync]\npoll_interval='1s'", "[sync]\npoll_interval='24h'", "[daemon]\ndebounce='100ms'", "[daemon]\ndebounce='1m'\nupdate_check=false"} {
		if _, err := Parse([]byte("schema=1\n[repo]\nurl='local.git'\n" + setting)); err != nil {
			t.Fatalf("rejected %s: %v", setting, err)
		}
	}
}
