package releasehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Metadata is bounded latest-release JSON or a conditional-cache hit. It does
// not expose assets, install operations or the self-update release protocol.
type Metadata struct {
	ETag        string
	Data        []byte
	NotModified bool
}

// Latest reads only the selected metadata endpoint. Unlike asset fetching, it
// refuses redirects, closes idle connections and preserves conditional ETags.
func (f Fetcher) Latest(ctx context.Context, address, etag, version string, limit int64) (result Metadata, err error) {
	if err = allowedURL(address); err != nil {
		return result, err
	}
	client := *f.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return result, err
	}
	request.Close = true
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "kitbag/"+version)
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode == http.StatusNotModified {
		return Metadata{NotModified: true}, nil
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("release metadata: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return result, err
	}
	if int64(len(data)) > limit {
		return result, errors.New("release metadata exceeds size limit")
	}
	return Metadata{Data: data, ETag: response.Header.Get("ETag")}, nil
}
