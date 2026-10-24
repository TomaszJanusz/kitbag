package manifest

import (
	"fmt"
	"strings"
)

// Error reports a rejected manifest at a one-based byte line and column. Key is
// a JSON pointer into the document; a missing key points to its enclosing table.
// Cause is retained for errors.Is (for example fs.ErrNotExist).
type Error struct {
	Cause   error
	File    string
	Key     string
	Message string
	Line    int
	Column  int
}

// Error returns a stable, positioned diagnostic.
func (e *Error) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s: %s", e.File, e.Line, e.Column, e.Key, e.Message)
}

// Unwrap returns the underlying filesystem or decoder error, if any.
func (e *Error) Unwrap() error { return e.Cause }

type (
	position struct{ line, column int }
	pathRef  struct{ key, value, role string }
	document struct {
		fsys      FileSystem
		file      string
		root      string
		positions map[string]position
		paths     []pathRef
	}
)

func (d *document) fail(key, message string, cause error) error {
	if d == nil {
		d = &document{file: "kitbag.toml"}
	}
	lookup := key
	pos, ok := d.positions[lookup]
	for !ok && lookup != "" {
		lookup = lookup[:strings.LastIndex(lookup, "/")]
		pos, ok = d.positions[lookup]
	}
	if !ok {
		pos = position{1, 1}
	}
	return &Error{File: d.file, Line: pos.line, Column: pos.column, Key: key, Message: message, Cause: cause}
}

func pointer(base, key string) string {
	return base + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
