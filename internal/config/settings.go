package config

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

// Values is a flattened TOML document; absent keys remain absent.
type Values map[string]any

// DecodeValues validates a whole file and retains explicit presence.
func DecodeValues(data []byte) (Values, error) {
	if _, err := Parse(data); err != nil {
		return nil, err
	}
	var tree map[string]any
	if err := toml.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	values := Values{}
	flatten("", tree, values)
	return values, nil
}

func flatten(prefix string, tree map[string]any, values Values) {
	for key, value := range tree {
		if nested, ok := value.(map[string]any); ok {
			flatten(prefix+key+".", nested, values)
		} else {
			values[prefix+key] = value
		}
	}
}

// Text formats strings as raw text and other values as TOML literals.
func Text(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, err := toml.Marshal(map[string]any{"value": value})
	if err != nil {
		return ""
	}
	_, literal, _ := strings.Cut(string(data), "= ")
	return strings.TrimSpace(literal)
}

// ParseValue accepts a raw string or a TOML literal for other types.
func ParseValue(key settingskey.Key, text string) (any, error) {
	if key.Type == "string" {
		return text, validateValue(key, text)
	}
	var tree map[string]any
	if err := toml.Unmarshal([]byte("value = "+text), &tree); err != nil {
		return nil, fmt.Errorf("%s: allowed %s: %w", key.Path, key.Constraint(), fs.ErrInvalid)
	}
	value, ok := tree["value"]
	if !ok || len(tree) != 1 {
		return nil, invalid(key.Path)
	}
	if err := validateValue(key, value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateValue(key settingskey.Key, value any) error {
	switch key.Type {
	case "integer":
		if _, ok := value.(int64); !ok {
			return invalid(key.Path + " (allowed: integer 1)")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return invalid(key.Path + " (allowed: true, false)")
		}
	case "array":
		return validateRoots(value)
	case "string":
		if _, ok := value.(string); !ok {
			return invalid(key.Path + " (allowed: string)")
		}
	}
	if key.Path == "repo.path" && !homePath(Text(value)) {
		return invalid(key.Path + " (allowed: " + key.Constraint() + ")")
	}
	return key.Validate(Text(value))
}

func validateRoots(value any) error {
	roots, ok := value.([]any)
	if !ok {
		return invalid("migration.legacy_roots (allowed: TOML array of unique home paths)")
	}
	seen := map[string]bool{}
	for _, root := range roots {
		text, valid := root.(string)
		if !valid || !homePath(text) || seen[text] {
			return invalid("migration.legacy_roots (allowed: unique home paths, each at most 4096 bytes)")
		}
		seen[text] = true
	}
	return nil
}

// EncodeValues validates the complete candidate, including required and paired keys.
func EncodeValues(values Values) ([]byte, error) {
	tree := map[string]any{}
	for path, value := range values {
		putValue(tree, strings.Split(path, "."), value)
	}
	data, err := toml.Marshal(tree)
	if err != nil {
		return nil, err
	}
	_, err = Parse(data)
	return data, err
}

func putValue(tree map[string]any, parts []string, value any) {
	if len(parts) == 1 {
		tree[parts[0]] = value
		return
	}
	child, ok := tree[parts[0]].(map[string]any)
	if !ok {
		child = map[string]any{}
		tree[parts[0]] = child
	}
	putValue(child, parts[1:], value)
}

// Resolve returns a winning value and its source, validating only that value.
func Resolve(env platform.Env, values Values, path, flag string) (string, string, error) {
	key, err := settingskey.Find(path)
	if err != nil {
		return "", "", err
	}
	environment, _ := env.LookupEnv(key.Env)
	value, source := key.Choose(flag, environment, Text(values[path]))
	if value == "" && path == "host" {
		value, err = env.Hostname()
		value, _, _ = strings.Cut(value, ".")
	}
	if err == nil && value != "" {
		_, err = ParseValue(key, value)
	}
	return value, source, err
}

// Effective applies registry environment/default values without persisting them.
// Host and mode are resolved by workflow/doctor so doctor can report bad overrides.
func Effective(env platform.Env, c Config, flags map[string]string) (Config, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return Config{}, err
	}
	var tree map[string]any
	if err = toml.Unmarshal(data, &tree); err != nil {
		return Config{}, err
	}
	prune(tree)
	values := Values{}
	flatten("", tree, values)
	for _, key := range settingskey.Registry() {
		if key.Path == "sync.mode" || key.Path == "host" {
			continue
		}
		value, _, resolveErr := Resolve(env, values, key.Path, flags[key.Flag])
		if resolveErr != nil {
			return Config{}, resolveErr
		}
		if value != "" {
			values[key.Path], err = ParseValue(key, value)
			if err != nil {
				return Config{}, err
			}
		}
	}
	return decodeEffective(env, values, c)
}

func decodeEffective(env platform.Env, values Values, c Config) (Config, error) {
	// Missing repository is permitted before init; the commands that use it reject it.
	if Text(values["repo.url"]) == "" {
		values["repo.url"] = "unconfigured"
	}
	encoded, err := EncodeValues(values)
	if err != nil {
		return Config{}, err
	}
	result, err := Parse(encoded)
	if c.Repo.URL == "" {
		if value, _ := env.LookupEnv("KITBAG_REPO_URL"); value == "" {
			result.Repo.URL = ""
		}
	}
	return result, err
}
