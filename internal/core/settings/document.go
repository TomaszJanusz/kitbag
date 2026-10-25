package settings

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

// Entries resolves every registry key for stored config bytes, the caller's
// environment and flags. Empty data is an absent file. It never touches disk,
// so a client holding bytes from the local API resolves exactly like the CLI.
func Entries(env platform.Env, data []byte, flags map[string]string) ([]Entry, error) {
	values, err := decode(data)
	if err != nil {
		return nil, err
	}
	return entries(env, values, flags)
}

func entries(env platform.Env, values config.Values, flags map[string]string) ([]Entry, error) {
	registry := settingskey.Registry()
	result := make([]Entry, 0, len(registry))
	for _, key := range registry {
		value, source, err := config.Resolve(env, values, key.Path, flags[key.Flag])
		if err != nil {
			return nil, err
		}
		result = append(result, Entry{Key: key, Value: value, Source: source})
	}
	return result, nil
}

// Changed returns the validated complete file after setting one key, or
// removing it when unset is true. It refuses comment-bearing files rather than
// silently discarding comments during encoding. The caller persists the result.
func Changed(data []byte, path, text string, unset bool) ([]byte, error) {
	key, err := settingskey.Find(path)
	if err != nil {
		return nil, err
	}
	if bytes.ContainsRune(data, '#') {
		return nil, fmt.Errorf("config contains # content; use config edit to preserve comments: %w", fs.ErrInvalid)
	}
	values, err := decode(data)
	if err != nil {
		return nil, err
	}
	if unset {
		delete(values, path)
	} else {
		value, parseErr := config.ParseValue(key, text)
		if parseErr != nil {
			return nil, parseErr
		}
		values[path] = value
	}
	if len(data) == 0 {
		values["schema"] = int64(1)
	}
	return config.EncodeValues(values)
}

func decode(data []byte) (config.Values, error) {
	if len(data) == 0 {
		return config.Values{}, nil
	}
	return config.DecodeValues(data)
}
