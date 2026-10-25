//go:build darwin || linux

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/update"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/releasehttp"
	"github.com/TomaszJanusz/kitbag/internal/releasesig"
)

func updateMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type localRelease struct {
	files  map[string][]byte
	verify func([]byte, []byte) error
}

func fakeRelease(t *testing.T, binary []byte) localRelease {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	updateMust(t, tw.WriteHeader(&tar.Header{Name: "kitbag", Mode: 0o755, Size: int64(len(binary))}))
	_, err := tw.Write(binary)
	updateMust(t, err)
	updateMust(t, tw.Close())
	updateMust(t, gz.Close())
	name := fmt.Sprintf("kitbag_2.0.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	checksum := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(out.Bytes()), name))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	updateMust(t, err)
	digest := sha256.Sum256(checksum)
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	updateMust(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	updateMust(t, err)
	verify, err := releasesig.Verifier(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	updateMust(t, err)
	return localRelease{verify: verify, files: map[string][]byte{
		"/latest/download/kitbag-update.json": []byte(`{"version":"2.0.0","channel":"stable"}`),
		"/latest/download/checksums.txt":      checksum,
		"/latest/download/checksums.txt.sig":  []byte(base64.StdEncoding.EncodeToString(signature)),
		"/latest/download/" + name:            out.Bytes(),
	}}
}

func (r localRelease) serve(w http.ResponseWriter, request *http.Request) {
	data, ok := r.files[request.URL.Path]
	if !ok {
		http.NotFound(w, request)
		return
	}
	if _, err := w.Write(data); err != nil {
		return
	}
}

func TestSelfUpdateLocalReleaseServer(t *testing.T) {
	t.Parallel()
	for _, tamper := range []bool{false, true} {
		t.Run(strconv.FormatBool(tamper), func(t *testing.T) {
			t.Parallel()
			release := fakeRelease(t, []byte("verified executable"))
			if tamper {
				for name := range release.files {
					if strings.HasSuffix(name, ".tar.gz") {
						release.files[name] = []byte("tampered")
					}
				}
			}
			server := httptest.NewServer(http.HandlerFunc(release.serve))
			defer server.Close()
			host := fake.New()
			binary := filepath.FromSlash("/bin/kitbag")
			updateMust(t, host.MkdirAll(filepath.Dir(binary), 0o755))
			updateMust(t, host.WriteAtomic(binary, []byte("old"), 0o755))
			updateMust(t, host.WriteAtomic(filepath.Join(filepath.Dir(binary), "kitbag.install-source"), []byte("install-script"), 0o644))
			execs := 0
			service := update.Service{
				FS: host, Binary: binary, Current: "1.0.0", OS: runtime.GOOS, Arch: runtime.GOARCH, BaseURL: server.URL, Fetch: (releasehttp.Fetcher{Client: server.Client()}).Fetch, Verify: release.verify,
				Probe: func(context.Context, string) (string, error) { return "kitbag 2.0.0", nil },
				Exec:  func(context.Context, string, []string) error { execs++; return nil },
			}
			var stdout bytes.Buffer
			cli := &commandLine{stdout: &stdout}
			cmd := cli.updateCommand(func() (update.Service, error) { return service, nil })
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			cmd.SetArgs([]string{"--channel", "stable"})
			err := cmd.ExecuteContext(t.Context())
			assertUpdateAttempt(t, stdout.String(), err, tamper, execs)
		})
	}
}

func TestSelfUpdateRealFilesystem(t *testing.T) {
	t.Parallel()
	original, err := os.Executable()
	updateMust(t, err)
	data, err := os.ReadFile(original) //nolint:gosec // G304: original is this test process executable, copied only to t.TempDir.
	updateMust(t, err)
	binary := filepath.Join(t.TempDir(), "update-subject")
	host := newHost()
	updateMust(t, host.WriteAtomic(binary, data, 0o755))
	updateMust(t, host.WriteAtomic(filepath.Join(filepath.Dir(binary), "kitbag.install-source"), []byte("release-archive"), 0o644))
	release := fakeRelease(t, data)
	server := httptest.NewServer(http.HandlerFunc(release.serve))
	defer server.Close()
	execs := 0
	service := update.Service{
		FS: host, Binary: binary, Current: "1.0.0", OS: runtime.GOOS, Arch: runtime.GOARCH, BaseURL: server.URL, Fetch: (releasehttp.Fetcher{Client: server.Client()}).Fetch, Verify: release.verify,
		Probe: func(ctx context.Context, path string) (string, error) {
			if _, statErr := host.Lstat(path + ".old"); statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
				return "", statErr
			}
			cmd := exec.CommandContext(ctx, path, "-test.run=^TestSelfUpdateProcess$")
			cmd.Env = []string{"KITBAG_UPDATE_TEST_HELPER=1"}
			output, runErr := cmd.Output()
			return strings.TrimSpace(string(output)), runErr
		},
		Exec: func(context.Context, string, []string) error { execs++; return nil },
	}
	result, err := service.Run(t.Context(), update.Options{Channel: "stable"})
	updateMust(t, err)
	if !result.Updated || execs != 1 {
		t.Fatalf("result %+v, execs %d", result, execs)
	}
	actual, err := host.ReadFile(binary)
	updateMust(t, err)
	if !bytes.Equal(actual, data) {
		t.Fatal("replacement bytes differ")
	}
	if _, err = host.Lstat(binary + ".old"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup: %v", err)
	}
}

func TestSelfUpdateProcess(t *testing.T) {
	t.Parallel()
	if helper, _ := newHost().LookupEnv("KITBAG_UPDATE_TEST_HELPER"); helper == "1" {
		if _, err := fmt.Fprint(os.Stdout, "kitbag 2.0.0"); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func TestSelfUpdateCommand(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"homebrew", "deb", "rpm", "aur", "go-install", "unknown"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			binary, err := platform.Executable()
			updateMust(t, err)
			host := fake.New()
			updateMust(t, host.MkdirAll(filepath.Dir(binary), 0o755))
			updateMust(t, host.WriteAtomic(filepath.Join(filepath.Dir(binary), "kitbag.install-source"), []byte(source), 0o644))
			var stdout, stderr bytes.Buffer
			code := run(runtime.GOOS, host, []string{"self-update", "--check"}, &stdout, &stderr)
			if code != 0 || !strings.Contains(stdout.String(), update.Instructions(source)) {
				t.Fatalf("exit %d: %s, %s", code, &stdout, &stderr)
			}
		})
	}
}

func assertUpdateAttempt(t *testing.T, output string, err error, tamper bool, execs int) {
	t.Helper()
	if (err != nil) != tamper {
		t.Fatalf("output %s, error %v", output, err)
	}
	if tamper {
		if execs != 0 {
			t.Fatal("executed tampered binary")
		}
		return
	}
	if execs != 1 || !strings.Contains(output, "updated kitbag to 2.0.0") {
		t.Fatalf("output %s, execs %d", output, execs)
	}
}
