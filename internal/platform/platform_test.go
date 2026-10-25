package platform_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestCheckOS(t *testing.T) {
	t.Parallel()

	for _, goos := range []string{"linux", "darwin"} {
		if err := platform.CheckOS(goos); err != nil {
			t.Errorf("CheckOS(%s) = %v, want nil", goos, err)
		}
	}
	err := platform.CheckOS("windows")
	var unsupported *platform.UnsupportedOSError
	if !errors.As(err, &unsupported) || !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("CheckOS(windows) = %v, want UnsupportedOSError matching ErrUnsupported", err)
	}
	if got, want := err.Error(), "Windows is not supported before 1.x"; got != want {
		t.Errorf("CheckOS(windows) message = %q, want %q", got, want)
	}
}

func TestTransientLinkReadsAreNarrow(t *testing.T) {
	t.Parallel()
	other := errors.New("other failure")
	tests := []struct {
		err  error
		name string
		goos string
		want bool
	}{
		{fmt.Errorf("read: %w", platform.ErrIsDir), "linux EISDIR", "linux", true},
		{fs.ErrInvalid, "linux EINVAL", "linux", false},
		{fs.ErrNotExist, "linux missing", "linux", false},
		{other, "linux other", "linux", false},
		{nil, "linux success", "linux", true},
		{fs.ErrInvalid, "darwin EINVAL", "darwin", true},
		{platform.ErrIsDir, "darwin EISDIR", "darwin", false},
		{other, "darwin other", "darwin", false},
		{platform.ErrIsDir, "windows EISDIR", "windows", false},
		{fs.ErrInvalid, "windows EINVAL", "windows", false},
		{platform.ErrIsDir, "unknown EISDIR", "freebsd", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tolerableLinkRead(transientLinkReadErrors(tt.goos), tt.err); got != tt.want {
				t.Errorf("tolerated = %v, want %v", got, tt.want)
			}
		})
	}
}
