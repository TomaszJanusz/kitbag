package daemonhost

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestReleaseETagCacheAndMetadataOnly(t *testing.T) {
	t.Parallel()
	now := time.Now()
	var requests atomic.Int32
	server := releaseServer(t, &requests)
	defer server.Close()
	host := fake.New()
	check := Releases{Host: host, StateDir: filepath.FromSlash("/state"), URL: server.URL + "/latest", Version: "1.1.0", Now: func() time.Time { return now }}
	for range 2 {
		assertReleaseAvailable(t, check, true)
	}
	if requests.Load() != 1 {
		t.Fatal("daily cache bypassed or assets downloaded", requests.Load())
	}
	now = now.Add(25 * time.Hour)
	assertReleaseAvailable(t, check, true)
	if requests.Load() != 2 {
		t.Fatal(requests.Load())
	}
	info, err := host.Lstat(filepath.Join(check.StateDir, "release-check.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	check.Version = "1.2.0"
	assertReleaseAvailable(t, check, false)
}

func releaseServer(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/latest" {
			t.Error("unexpected download/request", r.Method, r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"release"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"release"`)
		if _, err := fmt.Fprint(w, `{"tag_name":"v1.2.0","assets":[{"browser_download_url":"/asset"}],"tarball_url":"/tarball"}`); err != nil {
			t.Error(err)
		}
	}))
}

func TestReleaseFailures(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{"json": "{", "tag": `{"tag_name":"bad"}`, "prerelease": `{"tag_name":"v2.0.0","prerelease":true}`, "oversize": strings.Repeat("x", releaseLimit+1)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := fmt.Fprint(w, body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			check := Releases{Host: fake.New(), StateDir: filepath.FromSlash("/state"), URL: server.URL, Version: "1.0.0", Now: time.Now}
			if available, err := check.Check(t.Context()); err == nil || available {
				t.Fatal(available, err)
			}
		})
	}
	for _, status := range []int{http.StatusInternalServerError, http.StatusFound, http.StatusNotModified} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/asset")
				w.WriteHeader(status)
			}))
			defer server.Close()
			check := Releases{Host: fake.New(), StateDir: filepath.FromSlash("/state"), URL: server.URL, Version: "1.0.0", Now: time.Now}
			if available, err := check.Check(t.Context()); err == nil || available {
				t.Fatal(available, err)
			}
		})
	}
}

func TestNewerSemver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		tag, current string
		want         bool
	}{{"1.2.0", "1.1.9", true}, {"1.2.0", "1.2.0", false}, {"1.2.0", "1.2.1", false}, {"1.2.0", "1.2.0-rc.1", true}} {
		available, err := newer(tc.tag, tc.current)
		if err != nil || available != tc.want {
			t.Fatal(tc, available, err)
		}
	}
}

func assertReleaseAvailable(t *testing.T, check Releases, want bool) {
	t.Helper()
	available, err := check.Check(t.Context())
	if err != nil || available != want {
		t.Fatal(available, err)
	}
}
