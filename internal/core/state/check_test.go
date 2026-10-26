package state

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func checkFixture(t *testing.T) (*Store, *fake.Host, CheckRecord, string) {
	t.Helper()
	h := fake.New(fake.WithEnv(map[string]string{"HOME": filepath.FromSlash("/home/check")}))
	s, err := New(h, h, Options{})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 26, 17, 0, 0, 0, time.FixedZone("CET", 3600))
	r := CheckRecord{SchemaVersion: 1, At: at, NextAt: at.Add(time.Minute), Repository: filepath.FromSlash("/repo"), Outcome: "ok", Head: "abc123", IntervalSeconds: 60}
	return s, h, r, filepath.Join(filepath.Dir(s.path), "last-check.json")
}

func TestCheckLifecycleAndAtomicFailure(t *testing.T) {
	t.Parallel()
	s, h, r, path := checkFixture(t)
	if got, _, err := s.LoadCheck(); got != nil || err != nil {
		t.Fatal(got, err)
	}
	if err := s.SaveCheck(r); err != nil {
		t.Fatal(err)
	}
	before, err := h.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertCheckRoundTrip(t, s, r)
	info, err := h.Lstat(path)
	if err != nil || info.Mode().Perm() != statePerm {
		t.Fatal(info, err)
	}
	assertCheckAtomicFailure(t, s, h, r, path, before)
}

func assertCheckAtomicFailure(t *testing.T, s *Store, h *fake.Host, r CheckRecord, path string, before []byte) {
	t.Helper()
	var err error
	for _, stage := range []fake.WriteStage{fake.PartialWrite, fake.BeforeRename} {
		if err = h.InterruptNextWrite(path, stage); err != nil {
			t.Fatal(err)
		}
		r.Outcome, r.Category = "failed", "operation"
		if err = s.SaveCheck(r); err == nil {
			t.Fatal("interruption ignored")
		}
		after, readErr := h.ReadFile(path)
		if readErr != nil || !bytes.Equal(before, after) {
			t.Fatal("partial check replaced original", readErr)
		}
	}
}

func TestCheckRejectsUnsafeRecords(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*CheckRecord){
		func(r *CheckRecord) { r.SchemaVersion = 2 }, func(r *CheckRecord) { r.At = time.Time{} }, func(r *CheckRecord) { r.NextAt = r.At }, func(r *CheckRecord) { r.IntervalSeconds = 0 }, func(r *CheckRecord) { r.IntervalSeconds = 86401 }, func(r *CheckRecord) { r.Head = "https://secret" }, func(r *CheckRecord) { r.Head = strings.Repeat("a", 13) }, func(r *CheckRecord) { r.Outcome = "unknown" }, func(r *CheckRecord) { r.Category = "operation" }, func(r *CheckRecord) { r.Outcome = "failed"; r.Category = "raw secret" }, func(r *CheckRecord) { r.Repository = "relative" },
	} {
		s, _, r, _ := checkFixture(t)
		mutate(&r)
		if err := s.SaveCheck(r); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
}

func TestCheckPreservesUnreadableRecords(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{broken`, `{}`, `{"schema_version":2}`, strings.Repeat("x", checkLimit+1)} {
		s, h, r, path := checkFixture(t)
		if err := h.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			t.Fatal(err)
		}
		if err := h.WriteAtomic(path, []byte(raw), statePerm); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveCheck(r); err == nil {
			t.Fatal("corrupt record overwritten")
		}
	}
	s, h, r, path := checkFixture(t)
	if err := s.SaveCheck(r); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{` {}`, ` trailing`} {
		if err = h.WriteAtomic(path, append(data, []byte(suffix)...), statePerm); err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.LoadCheck(); err == nil {
			t.Fatal("trailing record accepted")
		}
	}
}

func assertCheckRoundTrip(t *testing.T, s *Store, r CheckRecord) {
	t.Helper()
	got, _, err := s.LoadCheck()
	if err != nil || got.At.Location() != time.UTC || got.NextAt.Location() != time.UTC || got.Outcome != "ok" || got.Stale(r.At.Add(3*time.Minute)) || !got.Stale(r.At.Add(3*time.Minute+time.Second)) {
		t.Fatalf("record=%+v err=%v", got, err)
	}
}

func TestCheckFractionalInterval(t *testing.T) {
	t.Parallel()
	s, _, r, _ := checkFixture(t)
	r.IntervalSeconds = 1.5
	if err := s.SaveCheck(r); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.LoadCheck()
	if err != nil {
		t.Fatal(err)
	}
	if got.Stale(r.At.Add(4500*time.Millisecond)) || !got.Stale(r.At.Add(4500*time.Millisecond+time.Nanosecond)) {
		t.Fatal("fractional interval truncated")
	}
}
