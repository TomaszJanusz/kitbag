package manifestedit

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Root-key and host-table edits used by the schema upgrade (ADR 12,
// "Migration: kitbag manifest upgrade").

const rootSchema, rootMinimum = "schema", "min_tool_version"

// upgradeEdit reports the operations of this file.
func upgradeEdit(op Op) bool {
	return slices.Contains([]Op{SetSchema, SetMinToolVersion, AddHostPatch, RemoveHostInstance, MoveHostInstance}, op)
}

func (l *layout) rewriteUpgrade(e Edit) ([]byte, error) {
	switch e.Op {
	case SetSchema:
		return l.setSchema(e)
	case SetMinToolVersion:
		return l.setMinToolVersion(e)
	case AddHostPatch:
		return l.addHostPatch(e)
	case RemoveHostInstance:
		return l.removeHostInstance(e)
	case MoveHostInstance:
		return l.moveHostInstance(e)
	case AddTarget, AddInstance, EditInstance, RemoveInstance, SetInstanceHosts:
	}
	return nil, fmt.Errorf("unexpected edit %q", e.Op)
}

// changeUpgrade applies an edit of this file to the decoded model.
func changeUpgrade(m *manifest.Manifest, e Edit) error {
	switch e.Op {
	case SetSchema:
		m.Schema = int(e.Schema)
	case SetMinToolVersion:
		m.MinToolVersion = e.Version
	case AddHostPatch, RemoveHostInstance, MoveHostInstance:
		return changeHost(m, e)
	case AddTarget, AddInstance, EditInstance, RemoveInstance, SetInstanceHosts:
		return fmt.Errorf("unexpected edit %q", e.Op)
	}
	return nil
}

func (l *layout) setSchema(e Edit) ([]byte, error) {
	keys := l.tables[0].keys
	i := l.key(keys, rootSchema)
	if i < 0 || keys[i].from < 0 {
		return nil, layoutError([]string{rootSchema})
	}
	return l.splice(keys[i].from, keys[i].to, strconv.FormatInt(e.Schema, 10)), nil
}

// setMinToolVersion replaces the root min_tool_version or adds it after the
// schema line.
func (l *layout) setMinToolVersion(e Edit) ([]byte, error) {
	keys := l.tables[0].keys
	if i := l.key(keys, rootMinimum); i >= 0 {
		if keys[i].from < 0 {
			return nil, layoutError([]string{rootMinimum})
		}
		return l.splice(keys[i].from, keys[i].to, quote(e.Version)), nil
	}
	i := l.key(keys, rootSchema)
	if i < 0 || keys[i].from < 0 {
		return nil, layoutError([]string{rootSchema})
	}
	at := keys[i].end
	return l.splice(at, at, l.lineBreak(at)+line(rootMinimum, quote(e.Version))+l.newline), nil
}

func (l *layout) splice(from, to int, text string) []byte {
	return slices.Concat(l.data[:from], []byte(text), l.data[to:])
}

func hostInstance(e Edit) []string {
	return []string{"hosts", e.Host, "targets", e.Target, instancesKey, e.Name}
}

// addHostPatch inserts [hosts.<h>.targets.<t>.instances.<n>] with the set
// keys after the host's last table, or at the end of the file.
func (l *layout) addHostPatch(e Edit) ([]byte, error) {
	if key := l.inlineUnder("hosts", e.Host, "targets", e.Target, instancesKey); key != nil {
		return nil, layoutError(key)
	}
	var lines []string
	if e.Enabled != nil {
		lines = append(lines, line("enabled", strconv.FormatBool(*e.Enabled)))
	}
	if e.Mode != "" {
		lines = append(lines, line("mode", quote(e.Mode)))
	}
	if e.Settings != nil {
		lines = append(lines, line("settings", strconv.FormatBool(*e.Settings)))
	}
	at := len(l.data)
	if related := l.within("hosts", e.Host); len(related) > 0 {
		at = l.contentEnd(related[len(related)-1])
	}
	return l.insert(at, header(hostInstance(e)...), lines...), nil
}

// hostBlocks returns the contiguous tables of a host instance table and its
// sub-tables, refusing inline or scattered layouts.
func (l *layout) hostBlocks(e Edit) ([]int, error) {
	path := hostInstance(e)
	if key := l.inlineUnder(path...); key != nil {
		return nil, layoutError(key)
	}
	blocks := l.within(path...)
	if l.find(path...) != blocks[0] {
		return nil, layoutError(path)
	}
	for k := 1; k < len(blocks); k++ {
		if blocks[k] != blocks[k-1]+1 {
			return nil, layoutError(l.tables[blocks[k]].path)
		}
	}
	return blocks, nil
}

// moveHostInstance moves a host instance table, its sub-tables and the
// comments attached above it to the end of its target, below
// [targets.<t>.instances.<n>] headers, with a hosts line after dir. Comment
// lines that only follow the moved tables stay where they are, and host
// headers left without keys or tables are removed.
func (l *layout) moveHostInstance(e Edit) ([]byte, error) {
	if err := l.editable(e.Target); err != nil {
		return nil, err
	}
	text, start, end, err := l.movedText(e)
	if err != nil {
		return nil, err
	}
	related := l.within("targets", e.Target)
	target := l.contentEnd(related[len(related)-1])
	if prev, ok := l.previousLine(start); (!ok || l.blank(prev, start)) && end < len(l.data) && l.blank(end, l.lineEnd(end)) {
		end = l.lineEnd(end) // Drop one of the two blank lines around the cut.
	}
	cut := &layout{data: slices.Concat(l.data[:start], l.data[end:]), newline: l.newline}
	if target > start {
		target -= end - start
	}
	after, err := scan(cut.insertText(target, string(text)))
	if err != nil {
		return nil, err
	}
	return after.removeEmptyHosts(hostInstance(e)), nil
}

// movedText returns the moved tables with target-level headers and the
// hosts line, and the byte range they came from.
func (l *layout) movedText(e Edit) ([]byte, int, int, error) {
	if len(l.within(hostInstance(e)...)) == 0 {
		return nil, 0, 0, layoutError(hostInstance(e))
	}
	blocks, err := l.hostBlocks(e)
	if err != nil {
		return nil, 0, 0, err
	}
	main := l.tables[blocks[0]]
	dir := l.key(main.keys, "dir")
	if dir < 0 || main.keys[dir].from < 0 {
		return nil, 0, 0, layoutError(append(slices.Clone(main.path), "dir"))
	}
	start, end := l.attached(main.start), l.lastLine(blocks[len(blocks)-1])
	var text []byte
	at := start
	for _, i := range blocks {
		t := l.tables[i]
		path := append([]string{"targets", e.Target, instancesKey, e.Name}, t.path[len(hostInstance(e)):]...)
		text = append(append(text, l.data[at:t.start]...), header(path...)...)
		at = t.close
		if i == blocks[0] {
			text = append(text, l.data[at:main.keys[dir].end]...)
			text = append(text, l.lineBreak(main.keys[dir].end)+line("hosts", hostList(e.Hosts))+l.newline...)
			at = main.keys[dir].end
		}
	}
	return append(text, l.data[at:end]...), start, end, nil
}

// lastLine returns the end of table i's last line that is neither blank nor
// a comment line.
func (l *layout) lastLine(i int) int {
	result := l.lineEnd(l.tables[i].start)
	for pos := result; pos < l.end(i); pos = l.lineEnd(pos) {
		if !l.blank(pos, l.lineEnd(pos)) && !l.comments[pos] {
			result = l.lineEnd(pos)
		}
	}
	return result
}

// removeEmptyHosts removes the headers above path that are left without keys
// and tables, keeping their comments.
func (l *layout) removeEmptyHosts(path []string) []byte {
	removed := map[int]bool{}
	for depth := len(path) - 1; depth >= 2; depth-- {
		if i := l.find(path[:depth]...); i >= 0 && len(l.tables[i].keys) == 0 && l.onlyRemoved(path[:depth], i, removed) {
			removed[i] = true
		}
	}
	return l.cutTables(removed, nil, true)
}

func hostList(hosts []string) string {
	quoted := make([]string, len(hosts))
	for i, host := range hosts {
		quoted[i] = quote(host)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// insertText adds a block of whole lines at a line start like insert does.
func (l *layout) insertText(at int, block string) []byte {
	text := l.lineBreak(at)
	if at > 0 {
		text += l.newline
	}
	text += block
	if !strings.HasSuffix(block, "\n") {
		text += l.newline
	}
	if at < len(l.data) && !l.blank(at, l.lineEnd(at)) {
		text += l.newline
	}
	return slices.Concat(l.data[:at], []byte(text), l.data[at:])
}

// removeHostInstance deletes a host instance table and its sub-tables. With
// keep, every comment in the removed bytes stays as a comment line.
func (l *layout) removeHostInstance(e Edit) ([]byte, error) {
	if len(l.within(hostInstance(e)...)) == 0 {
		return nil, layoutError(hostInstance(e))
	}
	return l.removeHostTables(e, true)
}

func (l *layout) removeHostTables(e Edit, keep bool) ([]byte, error) {
	blocks, err := l.hostBlocks(e)
	if err != nil {
		return nil, err
	}
	removed := map[int]bool{}
	for _, i := range blocks {
		removed[i] = true
	}
	path := hostInstance(e)
	for depth := len(path) - 1; depth >= 2; depth-- {
		if i := l.find(path[:depth]...); i >= 0 && len(l.tables[i].keys) == 0 && l.onlyRemoved(path[:depth], i, removed) {
			removed[i] = true
		}
	}
	return l.cutTables(removed, blocks, keep), nil
}

// cutTables deletes the removed tables like removeInstance does. With keep,
// or for tables outside blocks, every comment in the deleted bytes stays as a
// comment line.
func (l *layout) cutTables(removed map[int]bool, blocks []int, keep bool) []byte {
	out := slices.Clone(l.data)
	limit := len(l.data) // where the later, already cut range starts
	for _, i := range slices.Backward(slices.Sorted(maps.Keys(removed))) {
		from, to := l.removal(i)
		to = min(to, limit)
		kept := ""
		if keep || !slices.Contains(blocks, i) {
			kept = l.commentLines(from, to)
		}
		if kept != "" && to < len(l.data) && !l.blank(to, l.lineEnd(to)) {
			kept += l.newline // Keep the kept comments apart from the next table.
		}
		out = slices.Concat(out[:from], []byte(kept), out[to:])
		limit = from
	}
	return out
}

// removal returns the bytes removing table i deletes, as removeInstance
// computes them.
func (l *layout) removal(i int) (int, int) {
	from, to := l.tables[i].start, l.end(i)
	switch prev, ok := l.previousLine(from); {
	case i+1 == len(l.tables):
		from = l.trimBlankBefore(from)
	case !ok || l.blank(prev, from):
		to = l.attached(to)
	default:
		to = l.contentEnd(i)
	}
	return from, to
}

// onlyRemoved reports whether every table below prefix, other than i, is
// being removed.
func (l *layout) onlyRemoved(prefix []string, i int, removed map[int]bool) bool {
	for _, j := range l.within(prefix...) {
		if j != i && !removed[j] {
			return false
		}
	}
	return true
}

// commentLines returns every comment between from and to as its own line.
func (l *layout) commentLines(from, to int) string {
	var out []byte
	for _, c := range l.notes {
		if c.from >= from && c.to <= to {
			out = append(append(out, bytes.TrimRight(l.data[c.from:c.to], "\r")...), l.newline...)
		}
	}
	return string(out)
}

// changeHost applies a host-table edit to the decoded model.
func changeHost(m *manifest.Manifest, e Edit) error {
	ht := m.Hosts[e.Host].Targets[e.Target]
	instance, exists := ht.Instances[e.Name]
	switch {
	case e.Op == AddHostPatch && exists:
		return wrapped("host instance "+e.Host+"/"+e.Target+"/"+e.Name, ErrExists)
	case e.Op != AddHostPatch && !exists:
		return wrapped("host instance "+e.Host+"/"+e.Target+"/"+e.Name, ErrNotFound)
	}
	instances := maps.Clone(ht.Instances)
	if instances == nil {
		instances = map[string]manifest.Instance{}
	}
	if e.Op == AddHostPatch {
		instances[e.Name] = manifest.Instance{Enabled: e.Enabled, Mode: e.Mode, Settings: e.Settings}
	} else {
		delete(instances, e.Name)
	}
	if e.Op == MoveHostInstance {
		if err := moveToTarget(m, e, instance); err != nil {
			return err
		}
	}
	ht.Instances = instances
	setHostTarget(m, e.Host, e.Target, ht)
	return nil
}

func moveToTarget(m *manifest.Manifest, e Edit, instance manifest.Instance) error {
	target, ok := m.Targets[e.Target]
	if !ok {
		return wrapped("target "+e.Target, ErrNotFound)
	}
	if _, taken := target.Instances[e.Name]; taken {
		return wrapped("instance "+e.Target+"/"+e.Name, ErrExists)
	}
	instance.Hosts = slices.Clone(e.Hosts)
	target.Instances = maps.Clone(target.Instances)
	if target.Instances == nil {
		target.Instances = map[string]manifest.Instance{}
	}
	target.Instances[e.Name] = instance
	m.Targets[e.Target] = target
	return nil
}

// setHostTarget stores ht, dropping tables that became empty, as the editor
// removes headers left without keys or children.
func setHostTarget(m *manifest.Manifest, hostName, target string, ht manifest.HostTarget) {
	if len(ht.Instances) == 0 {
		ht.Instances = nil
	}
	host := m.Hosts[hostName]
	host.Targets = maps.Clone(host.Targets)
	if host.Targets == nil {
		host.Targets = map[string]manifest.HostTarget{}
	}
	host.Targets[target] = ht
	if ht.Enabled == nil && ht.Mode == "" && ht.Instances == nil {
		delete(host.Targets, target)
	}
	if len(host.Targets) == 0 {
		host.Targets = nil
	}
	setHost(m, hostName, host)
}

func setHost(m *manifest.Manifest, name string, host manifest.Host) {
	m.Hosts = maps.Clone(m.Hosts)
	if m.Hosts == nil {
		m.Hosts = map[string]manifest.Host{}
	}
	m.Hosts[name] = host
	if host.Publish == nil && host.Mode == "" && host.Targets == nil {
		delete(m.Hosts, name)
	}
	if len(m.Hosts) == 0 {
		m.Hosts = nil
	}
}
