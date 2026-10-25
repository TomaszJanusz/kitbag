package daemonhost

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestLogRotationAndAge(t *testing.T) {
	t.Parallel()
	host := fake.New()
	now := time.Time{}.Add(time.Hour)
	directory := filepath.FromSlash("/logs")
	logger := Log{Host: host, Directory: directory, Now: func() time.Time { return now }}
	mustLog(t, logger.Write("started", nil))
	path := filepath.Join(directory, "daemon.jsonl")
	for range logCount + 2 {
		mustLog(t, host.WriteAtomic(path, []byte(strings.Repeat("x", logLimit)), privateFile))
		mustLog(t, logger.Write("poll", fs.ErrNotExist))
	}
	names, err := host.ReadDir(directory)
	if err != nil || len(names) != logCount+1 {
		t.Fatal(names, err)
	}
	assertLogFailure(t, host, path)
	now = now.Add(logAge + time.Hour)
	mustLog(t, logger.Write("stopped", nil))
	names, err = host.ReadDir(directory)
	if err != nil || len(names) != 1 {
		t.Fatal(names, err)
	}
	data, err := host.ReadFile(path)
	if err != nil || strings.Contains(string(data), "poll") {
		t.Fatalf("expired logs retained: %s %v", data, err)
	}
}

func TestLogRefusesSymlink(t *testing.T) {
	t.Parallel()
	host := fake.New()
	path := filepath.FromSlash("/logs/daemon.jsonl")
	mustLog(t, host.MkdirAll(filepath.Dir(path), privateDir))
	mustLog(t, host.Link(filepath.FromSlash("/outside"), path))
	err := (Log{Host: host, Directory: filepath.Dir(path), Now: time.Now}).Write("poll", nil)
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
}

func mustLog(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertLogFailure(t *testing.T, host *fake.Host, path string) {
	t.Helper()
	data, err := host.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Action string
		Failed bool
	}
	if err = json.Unmarshal(data, &record); err != nil || record.Action != "poll" || !record.Failed {
		t.Fatal(record, err)
	}
}
