package platform_test

import (
	"errors"
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
