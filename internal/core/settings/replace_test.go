package settings

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestReplaceValidatesAndSharesLock(t *testing.T) {
	t.Parallel()
	s, host := fixture(t)
	seed(t, s)
	before, _, err := s.Read()
	must(t, err)
	if err = s.Replace([]byte("schema=2")); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid replacement=%v", err)
	}
	after, _, err := s.Read()
	must(t, err)
	if string(after) != string(before) {
		t.Fatal("invalid replacement changed bytes")
	}
	lock, err := host.TryLock(s.Filename + ".settings.lock")
	must(t, err)
	candidate := []byte("schema=1\n[repo]\nurl='replacement'\n")
	if err = s.Replace(candidate); !errors.Is(err, platform.ErrLocked) {
		t.Fatalf("replacement bypassed CLI settings lock: %v", err)
	}
	must(t, lock.Unlock())
	must(t, s.Replace(candidate))
	after, _, err = s.Read()
	must(t, err)
	if string(after) != string(candidate) {
		t.Fatal("replacement not persisted")
	}
}
