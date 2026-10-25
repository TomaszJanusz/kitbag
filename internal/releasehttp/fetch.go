// Package releasehttp confines release network I/O to a bounded transport.
package releasehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Fetcher uses an injected client, including its redirect and timeout policy.
type Fetcher struct{ Client *http.Client }

// Fetch accepts HTTPS, or loopback HTTP for local release mirrors and tests.
func (f Fetcher) Fetch(ctx context.Context, address string, limit int64) (data []byte, err error) {
	if err = allowedURL(address); err != nil {
		return nil, err
	}
	client := *f.Client
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		const redirectLimit = 10
		if len(via) >= redirectLimit {
			return errors.New("too many release redirects")
		}
		return allowedURL(request.URL.String())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release HTTP status %d", response.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, errors.New("release resource exceeds size limit")
	}
	return data, err
}

func allowedURL(address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}
	if parsed.User != nil {
		return errors.New("release URL must not contain credentials")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme != "http" {
		return errors.New("release URL must use HTTPS")
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return errors.New("HTTP release URL must use loopback")
	}
}

// New returns the production transport with a bounded request timeout.
func New() Fetcher {
	const timeout = 2 * time.Minute
	return Fetcher{Client: &http.Client{Timeout: timeout}}
}
