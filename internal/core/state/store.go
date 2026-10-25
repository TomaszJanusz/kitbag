package state

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	statePerm fs.FileMode = 0o600
	dirPerm   fs.FileMode = 0o700
)

// Store uses only host contracts. Construct it with New; WithLock defines the
// serialization boundary for an entire read/materialize/save operation.
type Store struct {
	fs       platform.FS
	path     string
	lockPath string
	options  Options
}

// New resolves the platform state directory without creating or changing files.
func New(hostFS platform.FS, env platform.Env, options Options) (*Store, error) {
	if options.HistoryLimit < 0 {
		return nil, fmt.Errorf("history limit: %w", fs.ErrInvalid)
	}
	if options.HistoryLimit == 0 {
		options.HistoryLimit = DefaultHistoryLimit
	}
	if options.Migration == nil {
		options.Migration = V1Migration{}
	}
	dirs, err := env.Dirs()
	if err != nil {
		return nil, fmt.Errorf("state directory: %w", err)
	}
	if err := validPath(dirs.State); err != nil {
		return nil, err
	}
	return &Store{fs: hostFS, path: filepath.Join(dirs.State, "state.json"), lockPath: filepath.Join(dirs.State, "state.lock"), options: options}, nil
}

// WithLock fails immediately with platform.ErrLocked and a clear diagnostic on
// contention. It holds the lock until fn returns (including its host operations).
// A leftover lock file is reused, never deleted: successful TryLock establishes
// that no live holder remains. Kernel locks need no PID/age-based stale breaking.
func (s *Store) WithLock(fn func(*Session) error) (err error) {
	if err = s.fs.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	lock, err := s.fs.TryLock(s.lockPath)
	if err != nil {
		if errors.Is(err, platform.ErrLocked) {
			return fmt.Errorf("state lock %s: another sync is running; retry after it exits: %w", s.lockPath, err)
		}
		return fmt.Errorf("state lock %s: %w", s.lockPath, err)
	}
	session := &Session{store: s, active: true}
	defer func() {
		session.mu.Lock()
		defer session.mu.Unlock()
		session.active = false
		err = errors.Join(err, lock.Unlock())
	}()
	return fn(session)
}

// Session is valid only inside WithLock. Load and Save serialize access, and
// reject use after the callback returns. Do not retain it or start background work.
type Session struct {
	store  *Store
	active bool
	mu     sync.Mutex
}

// Load returns a validated snapshot, or an empty current database if missing.
// Invalid, unsupported or unreadable state is never replaced with empty state.
func (s *Session) Load() (Database, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return Database{}, fs.ErrClosed
	}
	data, err := s.store.fs.ReadFile(s.store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Database{SchemaVersion: CurrentVersion, Targets: []Target{}, History: []AppliedPlan{}}, nil
	}
	if err != nil {
		return Database{}, fmt.Errorf("read state: %w", err)
	}
	return decode(data, s.store.options.Migration)
}

// Save validates, sorts and atomically replaces the snapshot, retaining the
// entries of the last configured N plans. The caller appends completed plans; this does not apply
// them or remove backups. It refuses to overwrite unreadable/newer on-disk state.
// A post-rename durability error may mean new state is already visible; reload
// before deciding whether to retry. Previous bytes survive pre-rename failure.
func (s *Session) Save(db Database) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return fs.ErrClosed
	}
	existing, err := s.readExisting()
	if err != nil {
		return err
	}
	if err = validate(db); err != nil {
		return err
	}
	db.Backups = retainBackups(db)
	db.History = retain(db.History, s.store.options.HistoryLimit)
	data, err := encode(db)
	if err != nil {
		return err
	}
	if err = s.preserveMigration(existing); err != nil {
		return err
	}
	if err = s.store.fs.WriteAtomic(s.store.path, data, statePerm); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// retainBackups preserves recovery metadata before history rotates. Newer
// unmanaged backups replace older records for the same destination; ordinary
// owned replacements do not change the original that unmanage should restore.
func retainBackups(db Database) []Change {
	byPath := make(map[string]Change)
	for _, change := range db.Backups {
		byPath[change.Path] = change
	}
	for _, entry := range db.History {
		for _, change := range entry.Changes {
			if prev := change.Previous; prev != nil && prev.Managed == nil && prev.BackupPath != "" {
				byPath[change.Path] = change
			}
		}
	}
	result := make([]Change, 0, len(byPath))
	for _, change := range byPath {
		result = append(result, change)
	}
	return result
}

// retain keeps the entries of the last limit plans; entries of one plan are
// contiguous because a plan runs under one lock.
func retain(history []AppliedPlan, limit int) []AppliedPlan {
	plans := 0
	for i := len(history) - 1; i >= 0; i-- {
		if i == len(history)-1 || history[i].PlanID() != history[i+1].PlanID() {
			plans++
		}
		if plans > limit {
			return history[i+1:]
		}
	}
	return history
}

func (s *Session) readExisting() ([]byte, error) {
	data, err := s.store.fs.ReadFile(s.store.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read existing state: %w", err)
	}
	if _, err := decode(data, s.store.options.Migration); err != nil {
		return nil, err
	}
	return data, nil
}

// preserveMigration keeps the original bytes before an explicit migrated Save.
// Never replace a conflicting archive; interrupted saves may safely retry.
func (s *Session) preserveMigration(data []byte) error {
	if data == nil {
		return nil
	}
	version, err := schemaVersion(data)
	if err != nil {
		return err
	}
	if version == CurrentVersion {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.store.path), fmt.Sprintf("state.v%d.json", version))
	previous, err := s.store.fs.ReadFile(path)
	if err == nil {
		if !bytes.Equal(previous, data) {
			return fmt.Errorf("migration archive %s differs: %w", path, fs.ErrExist)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read migration archive: %w", err)
	}
	if err := s.store.fs.WriteAtomic(path, data, statePerm); err != nil {
		return fmt.Errorf("preserve migration original: %w", err)
	}
	return nil
}

// Directory returns the protected state directory inside a live session. Apply
// uses it to refuse destinations that could replace its own state or lock.
func (s *Session) Directory() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return "", fs.ErrClosed
	}
	return filepath.Dir(s.store.path), nil
}
