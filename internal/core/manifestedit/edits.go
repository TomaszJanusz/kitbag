package manifestedit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const instancesKey = "instances"

func (l *layout) addTarget(e Edit) ([]byte, error) {
	if key := l.inlineUnder("targets"); key != nil {
		return nil, layoutError(key)
	}
	at := len(l.data)
	if related := l.within("targets"); len(related) > 0 {
		at = l.contentEnd(related[len(related)-1])
	}
	return l.insert(at, header("targets", e.Target), line("agent", quote(e.Agent))), nil
}

func (l *layout) addInstance(e Edit) ([]byte, error) {
	if err := l.editable(e.Target); err != nil {
		return nil, err
	}
	related := l.within("targets", e.Target)
	lines := []string{line("dir", quote(e.Dir))}
	if len(e.Hosts) > 0 {
		lines = append(lines, line("hosts", hostList(e.Hosts)))
	}
	if e.Settings != nil {
		lines = append(lines, line("settings", strconv.FormatBool(*e.Settings)))
	}
	at := l.contentEnd(related[len(related)-1])
	return l.insert(at, header("targets", e.Target, instancesKey, e.Name), lines...), nil
}

type splice struct {
	text     string
	from, to int
}

func (l *layout) editInstance(e Edit) ([]byte, error) {
	i, err := l.instance(e)
	if err != nil {
		return nil, err
	}
	keys := l.tables[i].keys
	dir, settings := l.key(keys, "dir"), l.key(keys, "settings")
	if dir < 0 || keys[dir].from < 0 || (settings >= 0 && keys[settings].from < 0) {
		return nil, layoutError(append(slices.Clone(l.tables[i].path), "dir"))
	}
	splices := l.settingsSplice(e, keys, dir, settings)
	if e.Dir != "" {
		splices = append(splices, splice{quote(e.Dir), keys[dir].from, keys[dir].to})
	}
	slices.SortFunc(splices, func(a, b splice) int { return b.from - a.from })
	out := slices.Clone(l.data)
	for _, s := range splices {
		out = slices.Concat(out[:s.from], []byte(s.text), out[s.to:])
	}
	return out, nil
}

// settingsSplice rewrites an existing switch, or adds settings = false after
// the dir line; an absent switch already means on.
func (l *layout) settingsSplice(e Edit, keys []keyValue, dir, settings int) []splice {
	switch {
	case e.Settings == nil:
		return nil
	case settings >= 0:
		return []splice{{strconv.FormatBool(*e.Settings), keys[settings].from, keys[settings].to}}
	case !*e.Settings:
		at := keys[dir].end
		return []splice{{l.lineBreak(at) + line("settings", "false") + l.newline, at, at}}
	}
	return nil
}

func (l *layout) key(keys []keyValue, name string) int {
	return slices.IndexFunc(keys, func(kv keyValue) bool { return slices.Equal(kv.path, []string{name}) })
}

// lineBreak returns the newline needed before inserting at the end of a last
// line without one.
func (l *layout) lineBreak(at int) string {
	if at > 0 && l.data[at-1] != '\n' {
		return l.newline
	}
	return ""
}

func (l *layout) removeInstance(e Edit, last bool) ([]byte, error) {
	if _, err := l.instance(e); err != nil {
		return nil, err
	}
	blocks := l.within("targets", e.Target, instancesKey, e.Name)
	if parent := l.find("targets", e.Target, instancesKey); last && parent >= 0 && len(l.tables[parent].keys) == 0 {
		blocks = append(blocks, parent)
		slices.Sort(blocks)
	}
	out := slices.Clone(l.data)
	for _, i := range slices.Backward(blocks) {
		from, to := l.tables[i].start, l.end(i)
		switch prev, ok := l.previousLine(from); {
		case i+1 == len(l.tables):
			from = l.trimBlankBefore(from)
		case !ok || l.blank(prev, from):
			to = l.attached(to)
		default:
			// No blank line precedes the table: keep the one that follows it.
			to = l.contentEnd(i)
		}
		out = slices.Concat(out[:from], out[to:])
	}
	return out, nil
}

// trimBlankBefore extends a removal that reaches the end of the file over
// the blank lines that separated it from the previous content.
func (l *layout) trimBlankBefore(pos int) int {
	for {
		prev, ok := l.previousLine(pos)
		if !ok || !l.blank(prev, pos) {
			return pos
		}
		pos = prev
	}
}

// editable refuses a target that is not its own [targets.<t>] table, or
// whose instances are written as inline tables or dotted keys.
func (l *layout) editable(target string) error {
	if l.find("targets", target) < 0 {
		return layoutError([]string{"targets", target})
	}
	if key := l.inlineUnder("targets", target, instancesKey); key != nil {
		return layoutError(key)
	}
	return nil
}

func (l *layout) instance(e Edit) (int, error) {
	if err := l.editable(e.Target); err != nil {
		return -1, err
	}
	path := []string{"targets", e.Target, instancesKey, e.Name}
	i := l.find(path...)
	if i < 0 {
		return -1, layoutError(path)
	}
	return i, nil
}

// insert adds a table block at a line start (or the end of the file),
// separated from the previous content by one blank line.
func (l *layout) insert(at int, head string, lines ...string) []byte {
	text := l.lineBreak(at)
	if at > 0 {
		text += l.newline
	}
	text += head + l.newline + strings.Join(lines, l.newline) + l.newline
	if at < len(l.data) && !l.blank(at, l.lineEnd(at)) {
		text += l.newline
	}
	return slices.Concat(l.data[:at], []byte(text), l.data[at:])
}

func header(path ...string) string {
	keys := make([]string, len(path))
	for i, key := range path {
		keys[i] = bareKey(key)
	}
	return "[" + strings.Join(keys, ".") + "]"
}

func line(key, value string) string { return bareKey(key) + " = " + value }

func bareKey(key string) string {
	if key != "" && strings.Trim(key, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == "" {
		return key
	}
	return quote(key)
}

// quote encodes a TOML basic string.
func quote(value string) string {
	out := []byte{'"'}
	for _, r := range value {
		switch {
		case r == '"' || r == '\\':
			out = append(out, '\\', byte(r))
		case r == utf8.RuneError || unicode.IsControl(r):
			out = fmt.Appendf(out, `\u%04X`, r)
		default:
			out = utf8.AppendRune(out, r)
		}
	}
	return string(append(out, '"'))
}
