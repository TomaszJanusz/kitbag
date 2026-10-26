package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

const checkLimit = 4096

// CheckRecord describes a completed daemon poll, independently of last sync.
// Category is a fixed error-catalog key; Head contains only a short hex hash.
type CheckRecord struct {
	At              time.Time `json:"at"`
	NextAt          time.Time `json:"next_at"`
	Repository      string    `json:"repository"`
	Outcome         string    `json:"outcome"`
	Category        string    `json:"category,omitempty"`
	Head            string    `json:"head,omitempty"`
	SchemaVersion   int       `json:"schema_version"`
	IntervalSeconds float64   `json:"interval_seconds"`
}

// Stale reports a poll older than three configured intervals.
func (r CheckRecord) Stale(now time.Time) bool {
	const intervals = 3
	return now.Sub(r.At) > intervals*time.Duration(r.IntervalSeconds*float64(time.Second))
}

func (r CheckRecord) validate() error {
	if r.SchemaVersion != 1 || r.At.IsZero() || !r.NextAt.After(r.At) {
		return invalid("last check time/version")
	}
	const maxInterval = 86400
	const maxHead = 12
	if r.IntervalSeconds < 1 || r.IntervalSeconds > maxInterval || len(r.Head) > maxHead {
		return invalid("last check bounds")
	}
	if strings.Trim(r.Head, "0123456789abcdef") != "" {
		return invalid("last check head")
	}
	if err := r.validateOutcome(); err != nil {
		return err
	}
	return validPath(r.Repository)
}

func (r CheckRecord) validateOutcome() error {
	if r.Outcome == "ok" && r.Category == "" {
		return nil
	}
	if r.Outcome == "failed" {
		switch r.Category {
		case "operation", "busy", "invalid", "conflict", "tool_too_old", "unsupported_os", "canceled":
			return nil
		}
	}
	return invalid("last check outcome/category")
}

// LoadCheck reads an atomic snapshot without acquiring the apply lock or writing.
func (s *Store) LoadCheck() (*CheckRecord, bool, error) {
	path := filepath.Join(filepath.Dir(s.path), "last-check.json")
	info, err := s.fs.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > checkLimit {
		return nil, false, invalid("last check file")
	}
	data, err := s.fs.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) > checkLimit {
		return nil, false, invalid("last check size")
	}
	return decodeCheck(data)
}

func decodeCheck(data []byte) (*CheckRecord, bool, error) {
	var err error
	var record CheckRecord
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&record); err != nil {
		return nil, false, invalid("last check JSON")
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, false, invalid("trailing last check data")
	}
	if err = record.validate(); err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

// SaveCheck atomically replaces the bounded record. The daemon lock owns writes.
// Invalid or newer existing records are preserved.
func (s *Store) SaveCheck(record CheckRecord) error {
	if _, _, err := s.LoadCheck(); err != nil {
		return err
	}
	if err := record.validate(); err != nil {
		return err
	}
	record.At, record.NextAt = record.At.UTC(), record.NextAt.UTC()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data)+1 > checkLimit {
		return invalid("last check size")
	}
	if err = s.fs.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return err
	}
	return s.fs.WriteAtomic(filepath.Join(filepath.Dir(s.path), "last-check.json"), append(data, '\n'), statePerm)
}
