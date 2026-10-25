package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// object retains jq's insertion order (including overwritten keys). A map
// would alphabetize settings on every sync and break byte parity with jq.
type (
	object []member
	member struct {
		value any
		key   string
	}
)

func (o object) get(key string) any {
	for _, entry := range o {
		if entry.key == key {
			return entry.value
		}
	}
	return nil
}

func (o *object) set(key string, value any) {
	for i := range *o {
		if (*o)[i].key == key {
			(*o)[i].value = value
			return
		}
	}
	*o = append(*o, member{value: value, key: key})
}

func (o object) without(key string) object {
	result := make(object, 0, len(o))
	for _, entry := range o {
		if entry.key != key {
			result = append(result, entry)
		}
	}
	return result
}

func parseObject(data []byte, empty bool) (object, error) {
	if empty && len(data) == 0 {
		return object{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("settings: invalid JSON: %w", err)
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("settings: expected one JSON object")
	}
	result, ok := value.(object)
	if !ok {
		return nil, errors.New("settings: not a JSON object; left alone")
	}
	return result, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		return decodeObject(decoder)
	case json.Delim('['):
		result := []any{}
		for decoder.More() {
			value, readErr := decodeValue(decoder)
			if readErr != nil {
				return nil, readErr
			}
			result = append(result, value)
		}
		_, err = decoder.Token()
		return result, err
	default:
		return token, nil
	}
}

func decodeObject(decoder *json.Decoder) (object, error) {
	result := object{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("settings: expected object key")
		}
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		result.set(key, value)
	}
	_, err := decoder.Token()
	return result, err
}

func equal(a, b any) bool {
	switch left := a.(type) {
	case object:
		return equalObjects(left, b)
	case []any:
		return equalArrays(left, b)
	case json.Number:
		return equalNumbers(left, b)
	default:
		return a == b
	}
}

func equalObjects(left object, value any) bool {
	right, ok := value.(object)
	if !ok || len(left) != len(right) {
		return false
	}
	for _, entry := range left {
		if !right.has(entry.key) || !equal(entry.value, right.get(entry.key)) {
			return false
		}
	}
	return true
}

func equalArrays(left []any, value any) bool {
	right, ok := value.([]any)
	if !ok || len(left) != len(right) {
		return false
	}
	for i := range left {
		if !equal(left[i], right[i]) {
			return false
		}
	}
	return true
}

func equalNumbers(left json.Number, value any) bool {
	right, ok := value.(json.Number)
	if !ok {
		return false
	}
	x, xOK := new(big.Rat).SetString(string(left))
	y, yOK := new(big.Rat).SetString(string(right))
	return xOK && yOK && x.Cmp(y) == 0
}

func (o object) has(key string) bool {
	for _, entry := range o {
		if entry.key == key {
			return true
		}
	}
	return false
}

// Typed accessors are used only after hook shape validation or a known merge.
func asObject(value any) object {
	if result, ok := value.(object); ok {
		return result
	}
	return nil
}

func asArray(value any) []any {
	if result, ok := value.([]any); ok {
		return result
	}
	return nil
}

func asString(value any) string {
	if result, ok := value.(string); ok {
		return result
	}
	return ""
}
