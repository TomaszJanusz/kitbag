package daemonhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/releasehttp"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const (
	releaseTimeout  = 10 * time.Second
	releaseLimit    = 64 * 1024
	releaseInterval = 24 * time.Hour
)

type releaseCache struct {
	Checked time.Time `json:"checked"`
	URL     string    `json:"url"`
	ETag    string    `json:"etag"`
	Tag     string    `json:"tag"`
}

// Releases only reads bounded release metadata. It has no asset/download API.
type Releases struct {
	Host                   platform.FS
	Now                    func() time.Time
	StateDir, URL, Version string
}

// Check caches ETags and successful checks across restarts for one day.
func (r Releases) Check(ctx context.Context) (available bool, err error) {
	path := filepath.Join(r.StateDir, "release-check.json")
	cache, err := r.load(path)
	if err != nil {
		return false, err
	}
	if r.fresh(cache) {
		return newer(cache.Tag, r.Version)
	}
	if cache.URL != r.URL {
		cache = releaseCache{URL: r.URL}
	}
	cache, err = r.fetch(ctx, cache)
	if err != nil {
		return false, err
	}
	available, err = newer(cache.Tag, r.Version)
	if err != nil {
		return false, err
	}
	cache.Checked = r.Now().UTC()
	data, err := json.Marshal(cache)
	if err != nil {
		return false, err
	}
	if err = r.Host.MkdirAll(r.StateDir, privateDir); err != nil {
		return false, err
	}
	return available, r.Host.WriteAtomic(path, data, privateFile)
}

func (r Releases) load(path string) (releaseCache, error) {
	data, err := r.Host.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return releaseCache{}, nil
	}
	if err != nil {
		return releaseCache{}, err
	}
	var cache releaseCache
	if len(data) > releaseLimit {
		return cache, fs.ErrInvalid
	}
	if err = json.Unmarshal(data, &cache); err != nil {
		return cache, err
	}
	return cache, nil
}

func readRelease(data []byte, etag string, cache releaseCache) (releaseCache, error) {
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if len(data) > releaseLimit {
		return cache, fs.ErrInvalid
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return cache, err
	}
	if release.Draft || release.Prerelease {
		return cache, fs.ErrInvalid
	}
	cache.Tag = strings.TrimPrefix(release.Tag, "v")
	cache.ETag = etag
	return cache, nil
}

func newer(tag, current string) (bool, error) {
	// Reuse the validated semantic-version contract, including prerelease order.
	m, err := manifest.Parse("release", []byte("schema=1\nmin_tool_version="+fmt.Sprintf("%q", tag)))
	if err != nil {
		return false, err
	}
	var old *manifest.VersionError
	err = m.CheckVersion(current)
	if errors.As(err, &old) {
		return true, nil
	}
	return false, err
}

func (r Releases) fetch(ctx context.Context, cache releaseCache) (result releaseCache, err error) {
	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()
	response, err := releasehttp.New().Latest(ctx, r.URL, cache.ETag, r.Version, releaseLimit)
	if err != nil {
		return cache, err
	}
	if response.NotModified {
		if cache.Tag == "" {
			return cache, fs.ErrInvalid
		}
		return cache, nil
	}
	return readRelease(response.Data, response.ETag, cache)
}

func (r Releases) fresh(cache releaseCache) bool {
	age := r.Now().Sub(cache.Checked)
	return cache.URL == r.URL && !cache.Checked.IsZero() && age >= 0 && age < releaseInterval
}
