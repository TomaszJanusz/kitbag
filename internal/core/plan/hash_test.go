package plan_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestLocalHashVersionAndDomains(t *testing.T) {
	t.Parallel()
	h := fake.New()
	directory := path("tree")
	if err := h.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	const emptyTreePreimage = "kitbag-tree-v1\x00[]"
	write(t, h, path("file"), emptyTreePreimage)
	treeHash := hashTree(t, h, directory)
	fileHash := hashTree(t, h, path("file"))
	if treeHash == fileHash {
		t.Fatal("file can impersonate a directory digest")
	}
	want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(emptyTreePreimage)))
	if treeHash != want {
		t.Fatalf("directory hash = %s; want %s", treeHash, want)
	}
	const filePreimage = "kitbag-file-v1\x00hello\r\n"
	want = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(filePreimage)))
	if got := plan.HashBytes([]byte("hello\r\n")); got != want {
		t.Fatalf("file hash = %s; want %s", got, want)
	}
	if plan.HashBytes([]byte("hello\r\n")) == plan.HashBytes([]byte("hello\n")) {
		t.Fatal("line ending normalization lost bytes")
	}
}

func TestDirectoryHashesIncludeNamesTypesAndEmptyDirectories(t *testing.T) {
	t.Parallel()
	h := fake.New()
	a, b := path("a"), path("b")
	write(t, h, filepath.Join(a, "nested", "file"), "data")
	write(t, h, filepath.Join(b, "nested", "file"), "data")
	before := hashTree(t, h, a)
	if before != hashTree(t, h, b) {
		t.Fatal("absolute path affected digest")
	}
	if err := h.MkdirAll(filepath.Join(b, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if before == hashTree(t, h, b) {
		t.Fatal("empty directory not counted")
	}
	if err := h.Rename(filepath.Join(a, "nested", "file"), filepath.Join(a, "nested", "renamed")); err != nil {
		t.Fatal(err)
	}
	if before == hashTree(t, h, a) {
		t.Fatal("name not counted")
	}
}

func TestHashTreeRejectsCaseAliases(t *testing.T) {
	t.Parallel()
	h := fake.New()
	write(t, h, path("tree", "Skill"), "one")
	write(t, h, path("tree", "ſkill"), "two")
	if _, err := plan.HashTree(h, path("tree")); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("case alias accepted: %v", err)
	}
}
