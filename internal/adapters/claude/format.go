package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// formatJSON matches jq's default two-space indentation, insertion order,
// literal Unicode/HTML, compact empty collections and one final newline.
func formatJSON(value any) ([]byte, error) {
	raw, err := appendJSON(nil, value)
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err = json.Indent(&pretty, raw, "", "  "); err != nil {
		return nil, err
	}
	return append(pretty.Bytes(), '\n'), nil
}

func appendJSON(out []byte, value any) ([]byte, error) {
	switch v := value.(type) {
	case object:
		return appendObject(out, v)
	case []any:
		return appendArray(out, v)
	case string:
		return quote(out, v), nil
	case json.Number:
		return append(out, string(v)...), nil
	case bool:
		return strconv.AppendBool(out, v), nil
	case nil:
		return append(out, "null"...), nil
	default:
		return nil, fmt.Errorf("settings: unsupported JSON value %T", value)
	}
}

func appendObject(out []byte, v object) ([]byte, error) {
	out = append(out, '{')
	for i, entry := range v {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(quote(out, entry.key), ':')
		var err error
		out, err = appendJSON(out, entry.value)
		if err != nil {
			return nil, err
		}
	}
	return append(out, '}'), nil
}

func appendArray(out []byte, v []any) ([]byte, error) {
	out = append(out, '[')
	for i, entry := range v {
		if i > 0 {
			out = append(out, ',')
		}
		var err error
		out, err = appendJSON(out, entry)
		if err != nil {
			return nil, err
		}
	}
	return append(out, ']'), nil
}

func quote(out []byte, value string) []byte {
	out = append(out, '"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out = append(out, '\\', byte(r))
		default:
			if r < ' ' || r == '\x7f' {
				out = appendControl(out, r)
			} else {
				out = append(out, string(r)...)
			}
		}
	}
	return append(out, '"')
}

func appendControl(out []byte, r rune) []byte {
	switch r {
	case '\b':
		return append(out, `\b`...)
	case '\f':
		return append(out, `\f`...)
	case '\n':
		return append(out, `\n`...)
	case '\r':
		return append(out, `\r`...)
	case '\t':
		return append(out, `\t`...)
	default:
		return fmt.Appendf(out, `\u%04x`, r)
	}
}
