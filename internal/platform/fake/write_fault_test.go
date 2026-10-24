package fake_test

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestWriteFaultOnlyAffectsChosenWrite(t *testing.T) {
	t.Parallel()
	host := fake.New()
	const path = "/state.json"
	if err := host.WriteAtomic(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := host.InterruptNextWrite(path, fake.PartialWrite); err != nil {
		t.Fatal(err)
	}
	if err := host.WriteAtomic("/other", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := host.Link("state.json", "/link"); err != nil {
		t.Fatal(err)
	}
	if err := host.WriteAtomic(path, []byte("replacement"), 0o600); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("injected failure = %v", err)
	}
	if got, err := host.ReadFile(path); err != nil || string(got) != "previous" {
		t.Fatalf("partial stage exposed: %q, %v", got, err)
	}
	if err := host.WriteAtomic(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFaultValidation(t *testing.T) {
	t.Parallel()
	host := fake.New()
	if err := host.InterruptNextWrite("relative", fake.PartialWrite); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	if err := host.InterruptNextWrite("/state.json", 0); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
}
