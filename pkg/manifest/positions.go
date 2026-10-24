package manifest

import (
	"strconv"

	"github.com/pelletier/go-toml/v2/unstable"
)

// index uses the same parser as decoding, including quoted/dotted keys, multiline
// arrays, inline tables and array-table indices. It runs only after valid TOML.
func (d *document) index(data []byte) {
	var parser unstable.Parser
	parser.Reset(data)
	arrays := map[string]int{}
	base := ""
	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			base = d.indexKey(&parser, node, "", arrays)
			if node.Kind == unstable.ArrayTable {
				index := arrays[base]
				arrays[base] = index + 1
				base = pointer(base, strconv.Itoa(index))
				d.positions[base] = nodePosition(&parser, node)
			}
		case unstable.KeyValue:
			key := d.indexKey(&parser, node, base, nil)
			d.indexValue(&parser, node.Value(), key)
		case unstable.Invalid, unstable.Comment, unstable.Key, unstable.Array, unstable.InlineTable,
			unstable.String, unstable.Bool, unstable.Float, unstable.Integer, unstable.LocalDate,
			unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
			// Only tables and key-value expressions occur at top level after decoding.
		}
	}
}

func (d *document) indexKey(parser *unstable.Parser, node *unstable.Node, base string, arrays map[string]int) string {
	keys := node.Key()
	for keys.Next() {
		key := keys.Node()
		base = pointer(base, string(key.Data))
		d.positions[base] = nodePosition(parser, key)
		if index, ok := arrays[base]; ok && !keys.IsLast() {
			base = pointer(base, strconv.Itoa(index-1))
		}
	}
	return base
}

func (d *document) indexValue(parser *unstable.Parser, node *unstable.Node, key string) {
	switch node.Kind {
	case unstable.InlineTable:
		children := node.Children()
		for children.Next() {
			entry := children.Node()
			child := d.indexKey(parser, entry, key, nil)
			d.indexValue(parser, entry.Value(), child)
		}
	case unstable.Array:
		children := node.Children()
		index := 0
		for children.Next() {
			child := pointer(key, strconv.Itoa(index))
			d.positions[child] = nodePosition(parser, children.Node())
			d.indexValue(parser, children.Node(), child)
			index++
		}
	case unstable.Invalid, unstable.Comment, unstable.Key, unstable.Table, unstable.ArrayTable,
		unstable.KeyValue, unstable.String, unstable.Bool, unstable.Float, unstable.Integer,
		unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		// Scalars have no descendant positions to index.
	}
}

func nodePosition(parser *unstable.Parser, node *unstable.Node) position {
	if node.Raw.Length != 0 {
		start := parser.Shape(node.Raw).Start
		return position{start.Line, start.Column}
	}
	// Composite nodes lack a raw range; their first child carries the location.
	if child := node.Child(); child != nil {
		return nodePosition(parser, child)
	}
	return position{1, 1}
}
