package manifestedit

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/pelletier/go-toml/v2/unstable"
)

// layout locates tables, keys and comment lines with the same parser
// pkg/manifest uses for diagnostics. Offsets index data.
type layout struct {
	comments map[int]bool
	notes    []note // every comment, in file order
	newline  string
	data     []byte
	tables   []table
}

// note is the byte range of one comment.
type note struct{ from, to int }

// table is a [header] block, or the root keys before the first header. Its
// bytes run from start to the next table's start; close is the offset just
// past the header's closing bracket.
type table struct {
	path  []string
	keys  []keyValue
	start int
	close int
	array bool
}

// keyValue is one key = value line. value is the byte range of a string,
// boolean or integer value; other kinds are never rewritten and have from = -1.
type keyValue struct {
	path     []string
	from, to int
	end      int // offset just past the line holding the value's end
}

func scan(data []byte) (*layout, error) {
	l := &layout{data: data, comments: map[int]bool{}, newline: "\n", tables: []table{{start: 0}}}
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		l.newline = "\r\n"
	}
	var parser unstable.Parser
	parser.KeepComments = true
	parser.Reset(data)
	for parser.NextExpression() {
		l.expression(&parser, parser.Expression())
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return l, nil
}

func (l *layout) expression(parser *unstable.Parser, node *unstable.Node) {
	switch node.Kind {
	case unstable.Comment:
		l.comment(int(node.Raw.Offset))
		l.note(int(node.Raw.Offset), int(node.Raw.Offset+node.Raw.Length))
	case unstable.Table, unstable.ArrayTable:
		path, first, last := keyPath(node)
		l.tables = append(l.tables, table{path: path, start: l.lineStart(bytes.LastIndexByte(l.data[:first], '[')), close: last + bytes.IndexByte(l.data[last:], ']') + 1, array: node.Kind == unstable.ArrayTable})
	case unstable.KeyValue:
		l.keyValue(parser, node)
	case unstable.Invalid, unstable.Key, unstable.Array, unstable.InlineTable, unstable.String,
		unstable.Bool, unstable.Float, unstable.Integer, unstable.LocalDate, unstable.LocalTime,
		unstable.LocalDateTime, unstable.DateTime:
		// Only comments, tables and key-value expressions occur at top level.
	}
	// A comment after a table header or key-value is chained to it.
	if next := node.Next(); next != nil && next.Valid() && next.Kind == unstable.Comment {
		l.note(int(next.Raw.Offset), int(next.Raw.Offset+next.Raw.Length))
	}
}

// keyPath returns a key's segments, the offset of the first one and the
// offset just past the last one.
func keyPath(node *unstable.Node) ([]string, int, int) {
	var path []string
	first, last := -1, -1
	keys := node.Key()
	for keys.Next() {
		key := keys.Node()
		if first < 0 {
			first = int(key.Raw.Offset)
		}
		last = int(key.Raw.Offset + key.Raw.Length)
		path = append(path, string(key.Data))
	}
	return path, first, last
}

// note records a comment's range; a comment that ends the input is reported
// without its leading #, so the range is widened back to it.
func (l *layout) note(from, to int) {
	for from > 0 && l.data[from] != '#' {
		from--
	}
	l.notes = append(l.notes, note{from, to})
}

func (l *layout) comment(offset int) {
	start := l.lineStart(offset)
	if len(bytes.TrimLeft(l.data[start:offset], " \t")) == 0 {
		l.comments[start] = true
	}
}

func (l *layout) keyValue(parser *unstable.Parser, node *unstable.Node) {
	path, _, _ := keyPath(node)
	kv := keyValue{path: path, from: -1, to: -1}
	value := node.Value()
	switch value.Kind {
	case unstable.String:
		kv.from = int(value.Raw.Offset)
		kv.to = kv.from + int(value.Raw.Length)
	case unstable.Bool, unstable.Integer:
		// Booleans and integers carry no raw range; their data references the input.
		r := parser.Range(value.Data)
		kv.from, kv.to = int(r.Offset), int(r.Offset+r.Length)
	case unstable.Invalid, unstable.Comment, unstable.Key, unstable.Table, unstable.ArrayTable,
		unstable.KeyValue, unstable.Array, unstable.InlineTable, unstable.Float,
		unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		// Other values are never rewritten.
	}
	if kv.to >= 0 {
		kv.end = l.lineEnd(kv.to)
	}
	last := &l.tables[len(l.tables)-1]
	last.keys = append(last.keys, kv)
}

// lineStart returns the offset of the line holding offset.
func (l *layout) lineStart(offset int) int {
	return bytes.LastIndexByte(l.data[:max(offset, 0)], '\n') + 1
}

// lineEnd returns the offset just past the newline ending offset's line, or
// the end of data.
func (l *layout) lineEnd(offset int) int {
	if i := bytes.IndexByte(l.data[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(l.data)
}

// end returns where table i's bytes end: the next table's start or EOF.
func (l *layout) end(i int) int {
	if i+1 < len(l.tables) {
		return l.tables[i+1].start
	}
	return len(l.data)
}

// previousLine returns the start of the line ending just before pos (a line
// start), and whether there is one.
func (l *layout) previousLine(pos int) (int, bool) {
	if pos == 0 {
		return 0, false
	}
	return l.lineStart(pos - 1), true
}

func (l *layout) blank(start, end int) bool {
	return len(bytes.Trim(l.data[start:end], " \t\r\n")) == 0
}

// attached returns the start of the comment lines directly above pos (no
// blank line between): they describe what follows and belong to it.
func (l *layout) attached(pos int) int {
	for {
		prev, ok := l.previousLine(pos)
		if !ok || !l.comments[prev] {
			return pos
		}
		pos = prev
	}
}

// contentEnd returns where table i's own content ends: before trailing blank
// lines and before the comments attached to the next table.
func (l *layout) contentEnd(i int) int {
	pos := l.end(i)
	if i+1 < len(l.tables) {
		pos = l.attached(pos)
	}
	for {
		prev, ok := l.previousLine(pos)
		if !ok || prev < l.tables[i].start || !l.blank(prev, pos) {
			return pos
		}
		pos = prev
	}
}

// find returns the index of the [header] table with exactly path, or -1.
func (l *layout) find(path ...string) int {
	return slices.IndexFunc(l.tables, func(t table) bool { return !t.array && t.start >= 0 && len(t.path) > 0 && slices.Equal(t.path, path) })
}

// within reports the tables whose path starts with prefix, in file order.
func (l *layout) within(prefix ...string) []int {
	var result []int
	for i, t := range l.tables {
		if len(t.path) >= len(prefix) && slices.Equal(t.path[:len(prefix)], prefix) {
			result = append(result, i)
		}
	}
	return result
}

// inlineUnder returns the first key, written by a table above prefix, whose
// full path reaches prefix: an inline table or dotted keys define it.
func (l *layout) inlineUnder(prefix ...string) []string {
	for _, t := range l.tables {
		if len(t.path) >= len(prefix) {
			continue
		}
		for _, kv := range t.keys {
			full := append(slices.Clone(t.path), kv.path...)
			if len(full) >= len(prefix) && slices.Equal(full[:len(prefix)], prefix) {
				return full
			}
		}
	}
	return nil
}
