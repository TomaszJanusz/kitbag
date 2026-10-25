// Package settings manages machine configuration through injected host contracts.
package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

const (
	directoryPerm fs.FileMode = 0o700
	filePerm      fs.FileMode = 0o600
)

// Service manages one explicitly resolved configuration file.
type Service struct {
	Host     platform.Host
	Flags    map[string]string
	Filename string
}

// Entry describes a discoverable effective setting.
type Entry struct {
	Value, Source string
	Key           settingskey.Key
}

// Read validates the stored file; an absent file is an empty configuration.
func (s Service) Read() ([]byte, config.Values, error) {
	data, err := s.Host.ReadFile(s.Filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, config.Values{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	values, err := config.DecodeValues(data)
	return data, values, err
}

// Get resolves one setting without modifying disk.
func (s Service) Get(path string) (Entry, error) {
	key, err := settingskey.Find(path)
	if err != nil {
		return Entry{}, err
	}
	_, values, err := s.Read()
	if err != nil {
		return Entry{}, err
	}
	value, source, err := config.Resolve(s.Host, values, path, s.Flags[key.Flag])
	return Entry{Key: key, Value: value, Source: source}, err
}

// List returns every registry key with its effective value and source.
func (s Service) List() ([]Entry, error) {
	_, values, err := s.Read()
	if err != nil {
		return nil, err
	}
	return entries(s.Host, values, s.Flags)
}

// Change sets a validated value or removes a key when unset is true, through
// Changed under the settings lock.
func (s Service) Change(path, text string, unset bool) error {
	if _, err := settingskey.Find(path); err != nil {
		return err
	}
	return s.locked(func() error {
		data, _, readErr := s.Read()
		if readErr != nil {
			return readErr
		}
		candidate, err := Changed(data, path, text, unset)
		if err != nil {
			return err
		}
		return s.Host.WriteAtomic(s.Filename, candidate, filePerm)
	})
}

func (s Service) locked(action func() error) (err error) {
	if err = s.Host.MkdirAll(filepath.Dir(s.Filename), directoryPerm); err != nil {
		return err
	}
	lock, err := s.Host.TryLock(s.Filename + ".settings.lock")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if info, statErr := s.Host.Lstat(s.Filename); statErr == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("config must be a regular file: %w", fs.ErrInvalid)
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}
	return action()
}

// Editor edits the draft path and returns only after the editor exits.
type Editor func(string) error

// Retry reports validation errors and asks whether to edit the draft again.
type Retry func(error) (bool, error)

// Edit validates a private draft after each editor run. Invalid edits and aborts
// never replace the original. A pre-existing draft is never overwritten.
func (s Service) Edit(editor Editor, retry Retry) error {
	return s.locked(func() error { return s.editLocked(editor, retry) })
}

func (s Service) editLocked(editor Editor, retry Retry) (err error) {
	original, readErr := s.Host.ReadFile(s.Filename)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	draft := s.Filename + ".edit.toml"
	if _, statErr := s.Host.Lstat(draft); !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("draft already exists or is inaccessible: %w", fs.ErrExist)
	}
	initial := original
	if len(initial) == 0 {
		initial = []byte("schema = 1\n[repo]\nurl = \"\"\n")
	}
	if err = s.Host.WriteAtomic(draft, initial, filePerm); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.Host.Remove(draft)) }()
	candidate, err := s.editDraft(draft, editor, retry)
	if err != nil || candidate == nil {
		return err
	}
	current, currentErr := s.Host.ReadFile(s.Filename)
	if changed(original, current, currentErr) {
		return fmt.Errorf("config changed while editing: %w", fs.ErrExist)
	}
	return s.Host.WriteAtomic(s.Filename, candidate, filePerm)
}

func (s Service) editDraft(draft string, editor Editor, retry Retry) ([]byte, error) {
	for {
		if err := editor(draft); err != nil {
			return nil, err
		}
		data, err := s.Host.ReadFile(draft)
		if err != nil {
			return nil, err
		}
		if _, err = config.Parse(data); err == nil {
			return data, nil
		}
		again, retryErr := retry(err)
		if retryErr != nil {
			return nil, retryErr
		}
		if !again {
			return nil, nil
		}
	}
}

// Display redacts sensitive fields for list/explain. Explicit get returns raw text.
func (e Entry) Display() string {
	if e.Key.Sensitive && e.Value != "" {
		return "[redacted]"
	}
	return strings.ReplaceAll(e.Value, "\n", `\n`)
}

func changed(original, current []byte, err error) bool {
	return !bytes.Equal(current, original) || (err != nil && !errors.Is(err, fs.ErrNotExist))
}
