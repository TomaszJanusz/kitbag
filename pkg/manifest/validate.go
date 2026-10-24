package manifest

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (d *document) validate(r *rule, value any, key string) error {
	switch r.kind {
	case "object":
		table, ok := value.(map[string]any)
		if !ok {
			return d.wrongType(key, value, "object")
		}
		return d.validateTable(r, table, key)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return d.wrongType(key, value, "array")
		}
		return d.validateArray(r, items, key)
	case "string":
		str, ok := value.(string)
		if !ok {
			return d.wrongType(key, value, "string")
		}
		return d.validateString(r, str, key)
	case "integer":
		return d.validateSchema(value, key)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return d.wrongType(key, value, "boolean")
		}
	}
	return nil
}

func (d *document) wrongType(key string, value any, want string) error {
	got := fmt.Sprintf("%T", value)
	if _, ok := value.(bool); ok {
		got = "boolean"
	}
	return d.fail(key, fmt.Sprintf("got %s, want %s", got, want), nil)
}

func (d *document) validateTable(r *rule, table map[string]any, key string) error {
	for _, required := range r.required {
		if _, ok := table[required]; !ok {
			return d.fail(key, "missing property '"+required+"'", nil)
		}
	}
	if len(table) < r.minimum {
		return d.fail(key, "table must not be empty", nil)
	}
	if err := d.tableNames(r, table, key); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(table)) {
		if err := d.validateProperty(r, name, table[name], key); err != nil {
			return err
		}
	}

	return d.conditional(table, key)
}

func (d *document) validateName(r *rule, name, key string) error {
	if !matches(r, name) {
		return d.fail(key, "invalid propertyName '"+name+"'", nil)
	}
	if r.pattern == namePattern || r.pattern == hostPattern {
		if err := portableName(name); err != nil {
			return d.fail(key, err.Error(), nil)
		}
	}
	return nil
}

func (d *document) validateArray(r *rule, items []any, key string) error {
	if len(items) < r.minimum {
		return d.fail(key, fmt.Sprintf("minItems: got %d, want %d", len(items), r.minimum), nil)
	}
	for i, item := range items {
		child := pointer(key, strconv.Itoa(i))
		if r.unique {
			for j := range i {
				if reflect.DeepEqual(item, items[j]) {
					return d.fail(child, fmt.Sprintf("items at %d and %d are equal", j, i), nil)
				}
			}
		}
		if err := d.validate(r.item, item, child); err != nil {
			return err
		}
	}
	return nil
}

func matches(r *rule, value string) bool {
	if len(r.values) > 0 && !slices.Contains(r.values, value) {
		return false
	}
	return r.pattern == "" || regexp.MustCompile(r.pattern).MatchString(value)
}

func (d *document) validateString(r *rule, value, key string) error {
	if !matches(r, value) {
		return d.stringMismatch(r, value, key)
	}
	size := utf8.RuneCountInString(value)
	if size < r.minimum || (r.maximum > 0 && size > r.maximum) {
		return d.fail(key, "invalid string length", nil)
	}
	if r.role == "" {
		return nil
	}
	if r.role == "template" && !strings.Contains(value, "{host}") {
		return d.fail(key, "does not match pattern: host template must contain {host}", nil)
	}
	for _, part := range strings.Split(strings.TrimPrefix(value, "~/"), "/") {
		if err := portableName(part); err != nil {
			return d.fail(key, err.Error(), nil)
		}
	}
	d.paths = append(d.paths, pathRef{key, value, r.role})
	return nil
}

func (d *document) stringMismatch(r *rule, value, key string) error {
	if len(r.values) > 0 {
		return d.fail(key, fmt.Sprintf("value must be one of %s; got %q", strings.Join(r.values, ", "), value), nil)
	}
	return d.fail(key, "does not match pattern "+r.pattern, nil)
}

func (d *document) unknown(key, name string, properties map[string]*rule) error {
	message := "additional properties '" + name + "' not allowed"
	if suggestion := suggest(name, slices.Sorted(maps.Keys(properties))); suggestion != "" {
		message += "; did you mean '" + suggestion + "'?"
	}
	return d.fail(key, message, nil)
}

func suggest(name string, candidates []string) string {
	best, score := "", 4
	for _, candidate := range candidates {
		distance := editDistance(name, candidate)
		if strings.TrimSuffix(name, "_file") == candidate {
			distance = 1
		}
		if distance < score {
			best, score = candidate, distance
		}
	}
	return best
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, ac := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1
		for j, bc := range b {
			cost := 0
			if ac != bc {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func (d *document) tableNames(r *rule, table map[string]any, key string) error {
	if r.names == nil || (r.names.pattern != namePattern && r.names.pattern != hostPattern) {
		return nil
	}
	seen := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(table)) {
		folded := foldName(name)
		if previous, ok := seen[folded]; ok {
			return d.fail(pointer(key, name), "case-insensitive name collision with "+previous, nil)
		}
		seen[folded] = name
	}
	return nil
}

func (d *document) validateSchema(value any, key string) error {
	n, ok := value.(int64)
	if !ok {
		return d.wrongType(key, value, "integer")
	}
	if n != 1 {
		return d.fail(key, "value must be 1 (unsupported schema; update kitbag or run an explicit migration)", nil)
	}
	return nil
}

func (d *document) validateProperty(r *rule, name string, value any, key string) error {
	child := pointer(key, name)
	next := r.item
	if r.properties != nil {
		next = r.properties[name]
	}
	if next == nil {
		return d.unknown(child, name, r.properties)
	}
	if r.names != nil {
		if err := d.validateName(r.names, name, child); err != nil {
			return err
		}
	}
	return d.validate(next, value, child)
}
