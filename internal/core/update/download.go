package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Limits bound both compressed downloads and expanded executables.
const (
	metadataLimit int64 = 16 << 10
	checksumLimit int64 = 1 << 20
	archiveLimit  int64 = 64 << 20
	binaryLimit   int64 = 128 << 20
)

type (
	artifact struct {
		version string
		binary  []byte
	}
	metadata struct {
		Version string `json:"version"`
		Channel string `json:"channel"`
	}
)

func (s Service) download(ctx context.Context, channel string) (artifact, error) {
	base := strings.TrimRight(s.BaseURL, "/") + "/latest/download"
	if channel == "nightly" {
		base = strings.TrimRight(s.BaseURL, "/") + "/download/nightly"
	}
	data, err := s.Fetch(ctx, base+"/kitbag-update.json", metadataLimit)
	if err != nil {
		return artifact{}, err
	}
	var meta metadata
	if err = json.Unmarshal(data, &meta); err != nil {
		return artifact{}, err
	}
	if err = validateRelease(meta, channel); err != nil {
		return artifact{}, err
	}
	return s.downloadArchive(ctx, base, meta)
}

func (s Service) downloadArchive(ctx context.Context, base string, meta metadata) (artifact, error) {
	checksums, err := s.signedChecksums(ctx, base)
	if err != nil {
		return artifact{}, err
	}
	name := fmt.Sprintf("kitbag_%s_%s_%s.tar.gz", meta.Version, s.OS, s.Arch)
	expected, err := checksum(checksums, name)
	if err != nil {
		return artifact{}, err
	}
	archive, err := s.Fetch(ctx, base+"/"+name, archiveLimit)
	if err != nil {
		return artifact{}, err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != expected {
		return artifact{}, errors.New("archive checksum mismatch")
	}
	binary, err := unpack(archive)
	return artifact{version: meta.Version, binary: binary}, err
}

func validateRelease(meta metadata, channel string) error {
	// CheckVersion validates the current version even without a minimum.
	if err := (&manifest.Manifest{}).CheckVersion(meta.Version); err != nil {
		return err
	}
	_, prerelease, _ := strings.Cut(strings.Split(meta.Version, "+")[0], "-")
	if meta.Channel != channel || channel == "stable" && prerelease != "" || channel == "nightly" && !strings.HasPrefix(prerelease, "nightly.") {
		return fmt.Errorf("release does not match requested channel %s: %w", channel, fs.ErrInvalid)
	}
	return nil
}

func checksum(data []byte, name string) (string, error) {
	var found string
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != path.Base(name) {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size || found != "" {
			return "", errors.New("invalid or duplicate archive checksum")
		}
		found = strings.ToLower(fields[0])
	}
	if found == "" {
		return "", errors.New("archive missing from signed checksums")
	}
	return found, nil
}

func (s Service) signedChecksums(ctx context.Context, base string) ([]byte, error) {
	data, err := s.Fetch(ctx, base+"/checksums.txt", checksumLimit)
	if err != nil {
		return nil, err
	}
	signature, err := s.Fetch(ctx, base+"/checksums.txt.sig", checksumLimit)
	if err != nil {
		return nil, err
	}
	if err = s.Verify(data, signature); err != nil {
		return nil, fmt.Errorf("release signature: %w", err)
	}
	return data, nil
}
