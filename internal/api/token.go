package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	tokenBytes              = 32
	tokenLength             = 64
	privateFile fs.FileMode = 0o600
	privateDir  fs.FileMode = 0o700
)

// SocketPath discovers the endpoint without creating directories.
func SocketPath(env platform.Env) (string, error) {
	dir, err := env.RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "api-v1.sock"), nil
}

func tokenPath(env platform.Env) (string, error) {
	dirs, err := env.Dirs()
	if err != nil {
		return "", err
	}
	return filepath.Join(dirs.State, "api-token"), nil
}

// LoadToken reads an existing private token. It never creates or repairs one.
func LoadToken(host platform.Host) (string, error) {
	path, err := tokenPath(host)
	if err != nil {
		return "", err
	}
	if err = privateTokenEntry(host, path); err != nil {
		return "", err
	}
	data, err := host.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) != tokenLength {
		return "", fs.ErrInvalid
	}
	if _, err = hex.DecodeString(string(data)); err != nil {
		return "", fs.ErrInvalid
	}

	return string(data), nil
}

func privateTokenEntry(host platform.FS, path string) error {
	fi, err := host.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != privateFile {
		return fs.ErrPermission
	}
	return nil
}

// EnsureToken creates a cryptographically random token once under a lock.
// Existing invalid/insecure tokens fail closed; errors never contain its bytes.
func EnsureToken(host platform.Host) (token string, err error) {
	path, err := tokenPath(host)
	if err != nil {
		return "", err
	}
	if err = host.MkdirAll(filepath.Dir(path), privateDir); err != nil {
		return "", err
	}
	lock, err := host.TryLock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	token, err = LoadToken(host)
	if !errors.Is(err, fs.ErrNotExist) {
		return token, err
	}
	bytes := make([]byte, tokenBytes)
	if _, err = rand.Read(bytes); err != nil {
		return "", err
	}
	encoded := []byte(hex.EncodeToString(bytes))
	token = string(encoded)
	err = host.WriteAtomic(path, encoded, privateFile)
	return token, err
}

func authorized(expected, supplied string) bool {
	// Hashing fixes the comparison length, including for an absent token.
	want, got := sha256.Sum256([]byte(expected)), sha256.Sum256([]byte(supplied))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
