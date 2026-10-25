package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

func schemaVersion(data []byte) (int, error) {
	var header struct {
		Version *int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, fmt.Errorf("state header: %w", err)
	}
	if header.Version == nil {
		return 0, fmt.Errorf("missing schema_version: %w", fs.ErrInvalid)
	}
	return *header.Version, nil
}

// V1Migration upgrades schema 1 losslessly: every schema 1 snapshot is a valid
// schema 2 snapshot whose history entries are each their own plan.
type V1Migration struct{}

// Migrate rewrites only schema_version, retaining every other member verbatim.
func (V1Migration) Migrate(previous []byte) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(previous, &document); err != nil {
		return nil, fmt.Errorf("schema 1 state: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("schema 1 state is not an object: %w", fs.ErrInvalid)
	}
	document["schema_version"] = json.RawMessage(strconv.Itoa(CurrentVersion))
	return json.Marshal(document)
}

func decode(data []byte, migration Migration) (Database, error) {
	version, err := schemaVersion(data)
	if err != nil {
		return Database{}, err
	}
	if version == CurrentVersion-1 && migration != nil {
		next, err := migration.Migrate(bytes.Clone(data))
		if err != nil {
			return Database{}, fmt.Errorf("migrate state: %w", err)
		}
		return decode(next, nil)
	}
	if version != CurrentVersion {
		return Database{}, fmt.Errorf("unsupported state schema_version %d (supported %d); use a compatible kitbag or explicit migration", version, CurrentVersion)
	}
	var db Database
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&db); err != nil {
		return Database{}, fmt.Errorf("decode state: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Database{}, fmt.Errorf("trailing state data: %w", fs.ErrInvalid)
	}
	if err := validate(db); err != nil {
		return Database{}, err
	}
	return db, nil
}

func encode(db Database) ([]byte, error) {
	// Clone via JSON so canonicalization cannot reorder the caller's nested slices.
	raw, err := json.Marshal(db)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}
	var cloned Database
	if err = json.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("clone state: %w", err)
	}
	canonicalize(&cloned)
	data, err := json.MarshalIndent(cloned, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode canonical state: %w", err)
	}
	return append(data, '\n'), nil
}

func canonicalize(db *Database) {
	if db.Targets == nil {
		db.Targets = []Target{}
	}
	if db.History == nil {
		db.History = []AppliedPlan{}
	}
	slices.SortFunc(db.Targets, func(a, b Target) int { return strings.Compare(a.Path, b.Path) })
	for i := range db.Targets {
		sortOwners(&db.Targets[i])
	}
	for i := range db.History {
		plan := &db.History[i]
		plan.AppliedAt = plan.AppliedAt.UTC()
		if plan.Changes == nil {
			plan.Changes = []Change{}
		}
		slices.SortFunc(plan.Changes, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
		for _, change := range plan.Changes {
			if change.Previous != nil && change.Previous.Managed != nil {
				sortOwners(change.Previous.Managed)
			}
		}
	}
}

func sortOwners(target *Target) {
	slices.SortFunc(target.Owners, func(a, b Owner) int {
		if cmp := strings.Compare(a.AgentInstance, b.AgentInstance); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Primitive, b.Primitive)
	})
}
