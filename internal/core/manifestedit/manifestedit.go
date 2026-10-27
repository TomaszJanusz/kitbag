// Package manifestedit changes agent instance declarations, host instance
// tables and the schema keys in kitbag.toml without re-encoding it: it inserts, rewrites or deletes only the affected
// lines and table blocks, found with the same go-toml/v2/unstable parser that
// pkg/manifest uses for positions, so comments, order and layout elsewhere stay
// byte for byte. Every result is reparsed with manifest.Parse and must decode
// to the old manifest plus exactly the intended change; layouts it cannot edit
// safely (inline tables, dotted keys defining a target or instance) are
// refused with ErrLayout instead (ADR 12, "Editing without losing the file").
package manifestedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Errors returned (wrapped) by Apply.
var (
	// ErrLayout means the manifest declares the edited table in a form this
	// editor does not change; the user edits it by hand.
	ErrLayout = errors.New("kitbag.toml declares it in a form KitBag cannot edit safely; edit it by hand")
	// ErrVerify means the edited bytes did not decode to the intended manifest.
	// It indicates an editor defect; nothing must be written.
	ErrVerify = errors.New("edited manifest does not match the intended change")
	// ErrExists means an added target or instance is already declared.
	ErrExists = errors.New("already declared")
	// ErrNotFound means an edited or removed instance, or its target, is not declared.
	ErrNotFound = errors.New("not declared")
)

// Op names a structural change.
type Op string

// Supported operations.
const (
	// AddTarget declares [targets.<Target>] with agent = Agent.
	AddTarget Op = "add_target"
	// AddInstance declares [targets.<Target>.instances.<Name>] with Dir and,
	// when Settings is set, the settings switch.
	AddInstance Op = "add"
	// EditInstance replaces Dir when it is not empty and sets Settings when it
	// is not nil. Settings on keeps an absent key absent (absent means on).
	EditInstance Op = "edit"
	// RemoveInstance deletes the instance table and its sub-tables.
	RemoveInstance Op = "remove"
	// SetSchema rewrites the root schema value to Schema.
	SetSchema Op = "set_schema"
	// SetMinToolVersion sets the root min_tool_version to Version, adding the
	// key after schema when it is absent.
	SetMinToolVersion Op = "set_min_tool_version"
	// AddHostPatch declares [hosts.<Host>.targets.<Target>.instances.<Name>]
	// with the Enabled, Mode and Settings that are set (a schema-2 patch).
	AddHostPatch Op = "add_host_patch"
	// RemoveHostInstance deletes a host instance table and its sub-tables;
	// every comment in them stays as a comment line.
	RemoveHostInstance Op = "remove_host_instance"
	// MoveHostInstance moves a host instance table, its sub-tables and its
	// comments to [targets.<Target>.instances.<Name>] with hosts = Hosts.
	MoveHostInstance Op = "move_host_instance"
)

// Edit is one change. Fields other than Op and Target apply as Op documents.
// AddInstance also writes Hosts when it is set.
type Edit struct {
	Settings *bool
	Enabled  *bool
	Op       Op
	Target   string
	Name     string
	Agent    string
	Dir      string
	Host     string
	Mode     string
	Version  string
	Hosts    []string
	Schema   int64
}

// Apply applies edits in order to data, a manifest that manifest.Parse
// accepts, and returns the new bytes after verifying them. filename only
// labels diagnostics.
func Apply(filename string, data []byte, edits ...Edit) ([]byte, error) {
	intent, err := manifest.Parse(filename, data)
	if err != nil {
		return nil, err
	}
	out := data
	for _, e := range edits {
		if err = change(intent, e); err != nil {
			return nil, err
		}
		if out, err = rewrite(out, intent, e); err != nil {
			return nil, err
		}
	}
	if err = verify(filename, out, intent); err != nil {
		return nil, err
	}
	return out, nil
}

// change applies e to the decoded model, which becomes the expected result.
func change(m *manifest.Manifest, e Edit) error {
	if upgradeEdit(e.Op) {
		return changeUpgrade(m, e)
	}
	target, ok := m.Targets[e.Target]
	if e.Op == AddTarget {
		if ok {
			return fmt.Errorf("target %s: %w", e.Target, ErrExists)
		}
		m.Targets = maps.Clone(m.Targets)
		if m.Targets == nil {
			m.Targets = map[string]manifest.Target{}
		}
		m.Targets[e.Target] = manifest.Target{Agent: e.Agent}
		return nil
	}
	if !ok {
		return fmt.Errorf("target %s: %w", e.Target, ErrNotFound)
	}
	instances, err := changeInstances(target.Instances, e)
	if err != nil {
		return err
	}
	target.Instances = instances
	if len(instances) == 0 {
		target.Instances = nil
	}
	m.Targets[e.Target] = target
	return nil
}

func changeInstances(current map[string]manifest.Instance, e Edit) (map[string]manifest.Instance, error) {
	instance, exists := current[e.Name]
	if err := presence(e, exists); err != nil {
		return nil, err
	}
	result := maps.Clone(current)
	if result == nil {
		result = map[string]manifest.Instance{}
	}
	switch e.Op {
	case AddInstance:
		result[e.Name] = manifest.Instance{Dir: e.Dir, Settings: e.Settings, Hosts: e.Hosts}
	case EditInstance:
		result[e.Name] = edited(instance, e)
	case RemoveInstance:
		delete(result, e.Name)
	case AddTarget, SetSchema, SetMinToolVersion, AddHostPatch, RemoveHostInstance, MoveHostInstance:
		return nil, fmt.Errorf("unexpected edit %q", e.Op)
	default:
		return nil, fmt.Errorf("unknown edit %q", e.Op)
	}
	return result, nil
}

func presence(e Edit, exists bool) error {
	switch {
	case e.Op == AddInstance && exists:
		return fmt.Errorf("instance %s/%s: %w", e.Target, e.Name, ErrExists)
	case e.Op != AddInstance && !exists:
		return fmt.Errorf("instance %s/%s: %w", e.Target, e.Name, ErrNotFound)
	}
	return nil
}

func edited(instance manifest.Instance, e Edit) manifest.Instance {
	if e.Dir != "" {
		instance.Dir = e.Dir
	}
	if e.Settings != nil && (!*e.Settings || instance.Settings != nil) {
		value := *e.Settings
		instance.Settings = &value
	}
	return instance
}

func verify(filename string, out []byte, intent *manifest.Manifest) error {
	got, err := manifest.Parse(filename, out)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	want, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	have, err := json.Marshal(got)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, have) {
		return ErrVerify
	}
	return nil
}

func rewrite(data []byte, intent *manifest.Manifest, e Edit) ([]byte, error) {
	l, err := scan(data)
	if err != nil {
		return nil, err
	}
	switch e.Op {
	case AddTarget:
		return l.addTarget(e)
	case AddInstance:
		return l.addInstance(e)
	case EditInstance:
		return l.editInstance(e)
	case RemoveInstance:
		return l.removeInstance(e, len(intent.Targets[e.Target].Instances) == 0)
	case SetSchema, SetMinToolVersion, AddHostPatch, RemoveHostInstance, MoveHostInstance:
		return l.rewriteUpgrade(e)
	default:
		return nil, fmt.Errorf("unknown edit %q", e.Op)
	}
}

func wrapped(subject string, err error) error { return fmt.Errorf("%s: %w", subject, err) }

func layoutError(path []string) error {
	return fmt.Errorf("%s: %w", strings.Join(path, "."), ErrLayout)
}
