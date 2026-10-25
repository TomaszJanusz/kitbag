package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	configDirectoryPerm fs.FileMode = 0o700
	configFilePerm      fs.FileMode = 0o600
)

// Encode returns validated TOML, omitting unset optional configuration fields.
func Encode(c Config) ([]byte, error) {
	data, err := toml.Marshal(c)
	if err != nil {
		return nil, err
	}
	var tree map[string]any
	if err = toml.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	prune(tree)
	data, err = toml.Marshal(tree)
	if err != nil {
		return nil, err
	}
	_, err = Parse(data)
	return data, err
}

func prune(tree map[string]any) {
	for key, value := range tree {
		switch item := value.(type) {
		case string:
			if item == "" {
				delete(tree, key)
			}
		case map[string]any:
			prune(item)
			if len(item) == 0 {
				delete(tree, key)
			}
		}
	}
}

// Create atomically installs validated configuration without replacing any
// existing entry. The caller serializes other setup work; the staging lock
// protects concurrent creators of this file. A leftover lock file is reused.
func Create(host platform.FS, filename string, c Config) (err error) {
	data, err := Encode(c)
	if err != nil {
		return err
	}
	if err = host.MkdirAll(filepath.Dir(filename), configDirectoryPerm); err != nil {
		return err
	}
	lock, err := host.TryLock(filename + ".init.lock")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	stage := filename + ".init-new"
	if err = absent(host, filename); err != nil {
		return err
	}
	if err = absent(host, stage); err != nil {
		return err
	}
	if err = host.WriteAtomic(stage, data, configFilePerm); err != nil {
		return err
	}
	defer func() {
		cleanup := host.Remove(stage)
		if !errors.Is(cleanup, fs.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	return host.Rename(stage, filename)
}

func absent(host platform.FS, filename string) error {
	_, err := host.Lstat(filename)
	if err == nil {
		return fmt.Errorf("%s already exists: %w", filename, fs.ErrExist)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
