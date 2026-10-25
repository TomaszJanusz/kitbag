package fake_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestForeignSocketPeer(t *testing.T) {
	t.Parallel()
	host := fake.New()
	path := filepath.FromSlash("/run/kitbag/api.sock")
	listener, err := host.ListenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	if _, err = host.DialLocalAs(t.Context(), path, false); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("foreign peer: %v", err)
	}
}
