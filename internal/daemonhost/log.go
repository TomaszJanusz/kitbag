package daemonhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	privateDir  fs.FileMode = 0o700
	privateFile fs.FileMode = 0o600
	logLimit                = 1024 * 1024
	logCount                = 3
	logAge                  = 7 * 24 * time.Hour
)

// Log writes bounded JSON lines atomically. It is owned by the scheduler, with no
// concurrent writers. Errors are classified without logging paths or credentials.
type Log struct {
	Host      platform.FS
	Now       func() time.Time
	Directory string
}

// Write appends one record, rotating at 1 MiB, three archives, or seven days.
func (l Log) Write(action string, failure error) error {
	if err := l.Host.MkdirAll(l.Directory, privateDir); err != nil {
		return err
	}
	path := filepath.Join(l.Directory, "daemon.jsonl")
	if err := l.expire(path); err != nil {
		return err
	}
	data, err := l.Host.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	record := struct {
		At     time.Time `json:"at"`
		Action string    `json:"action"`
		Failed bool      `json:"failed"`
	}{l.Now().UTC(), action, failure != nil}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data)+len(line)+1 > logLimit {
		if err = l.rotate(path); err != nil {
			return err
		}
		data = nil
	}
	return l.Host.WriteAtomic(path, append(append(data, line...), '\n'), privateFile)
}

func (l Log) rotate(path string) error {
	for i := logCount; i >= 1; i-- {
		dst := fmt.Sprintf("%s.%d", path, i)
		if err := l.Host.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		src := path
		if i > 1 {
			src = fmt.Sprintf("%s.%d", path, i-1)
		}
		if err := l.Host.Rename(src, dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (l Log) expire(path string) error {
	for i := 0; i <= logCount; i++ {
		name := path
		if i > 0 {
			name = fmt.Sprintf("%s.%d", path, i)
		}
		info, err := l.Host.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fs.ErrInvalid
		}
		if l.Now().Sub(info.ModTime()) > logAge {
			if err = l.Host.Remove(name); err != nil {
				return err
			}
		}
	}
	return nil
}
