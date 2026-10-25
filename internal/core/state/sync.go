package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"time"
)

// SyncRecord is the most recent sync attempt that acquired the state lock.
// Result is the stable apply exit code; no file bytes or error strings are stored.
type SyncRecord struct {
	At            time.Time `json:"at"`
	Repository    string    `json:"repository"`
	SchemaVersion int       `json:"schema_version"`
	Result        int       `json:"result"`
	Changed       int       `json:"changed"`
}

func (r SyncRecord) validate() error {
	if r.SchemaVersion != 1 || r.At.IsZero() || r.Result < 0 || r.Result > 5 || r.Changed < 0 {
		return invalid("last sync record")
	}
	return validPath(r.Repository)
}

// LoadSync reads the independent, versioned last-sync.json; absence means never.
func (s *Session) LoadSync() (SyncRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return SyncRecord{}, false, fs.ErrClosed
	}
	return s.loadSync()
}

func (s *Session) loadSync() (SyncRecord, bool, error) {
	data, err := s.store.fs.ReadFile(filepath.Join(filepath.Dir(s.store.path), "last-sync.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return SyncRecord{}, false, nil
	}
	if err != nil {
		return SyncRecord{}, false, fmt.Errorf("last sync: %w", err)
	}
	var record SyncRecord
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&record); err != nil {
		return SyncRecord{}, false, fmt.Errorf("last sync: %w", err)
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return SyncRecord{}, false, invalid("trailing last sync data")
	}
	if err = record.validate(); err != nil {
		return SyncRecord{}, false, err
	}
	return record, true, nil
}

// SaveSync atomically records an attempt without rewriting ownership or history.
// An unreadable/newer existing record is never overwritten.
func (s *Session) SaveSync(record SyncRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return fs.ErrClosed
	}
	if _, _, err := s.loadSync(); err != nil {
		return err
	}
	if err := record.validate(); err != nil {
		return err
	}
	record.At = record.At.UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return s.store.fs.WriteAtomic(filepath.Join(filepath.Dir(s.store.path), "last-sync.json"), append(data, '\n'), statePerm)
}
