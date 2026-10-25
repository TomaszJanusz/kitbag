package releasehttp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/truncated":
			w.Header().Set("Content-Length", "100")
			if _, err := w.Write([]byte("short")); err != nil {
				return
			}
		case "/missing":
			http.NotFound(w, r)
		case "/redirect":
			http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			if _, err := w.Write([]byte("release")); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	fetcher := Fetcher{Client: server.Client()}
	for _, tc := range []struct {
		path  string
		limit int64
		fail  bool
	}{
		{"/", 10, false}, {"/", 2, true}, {"/truncated", 128, true}, {"/missing", 10, true}, {"/redirect", 10, true}, {"/loop", 10, true},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			t.Parallel()
			data, err := fetcher.Fetch(t.Context(), server.URL+tc.path, tc.limit)
			if (err != nil) != tc.fail {
				t.Fatalf("data %q, error %v", data, err)
			}
		})
	}
}

func TestURLPolicy(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"http://example.invalid", "file:///tmp/file", "https://user:secret@example.invalid", "%"} {
		if err := allowedURL(address); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	for _, address := range []string{"https://example.invalid", "http://localhost:8000", "http://127.0.0.1", "http://[::1]"} {
		if err := allowedURL(address); err != nil {
			t.Fatal(err)
		}
	}
	if New().Client.Timeout <= 0 {
		t.Fatal("unbounded transport")
	}
}
