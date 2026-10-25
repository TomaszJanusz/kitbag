package update

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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/releasesig"
)

type fixture struct {
	host    *fake.Host
	files   map[string][]byte
	key     *ecdsa.PrivateKey
	service Service
	execs   int
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{host: fake.New(), files: map[string][]byte{}}
	binary := filepath.FromSlash("/bin/kitbag")
	must(t, f.host.MkdirAll(filepath.Dir(binary), binaryPerm))
	must(t, f.host.WriteAtomic(binary, []byte("old executable"), binaryPerm))
	must(t, f.host.WriteAtomic(filepath.Join(filepath.Dir(binary), "kitbag.install-source"), []byte("release-archive\n"), binaryPerm))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	f.key = key
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	must(t, err)
	verify, err := releasesig.Verifier(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	must(t, err)
	f.service = Service{
		FS: f.host, Binary: binary, Current: "1.0.0", OS: "linux", Arch: "amd64", BaseURL: "https://release.invalid", Verify: verify,
		Probe: func(context.Context, string) (string, error) { return "kitbag 2.0.0", nil },
		Exec: func(_ context.Context, path string, args []string) error {
			f.execs++
			if path != binary || len(args) != 1 || args[0] != "version" {
				t.Errorf("re-exec: %s %v", path, args)
			}
			return nil
		},
	}
	f.service.Fetch = func(_ context.Context, address string, limit int64) ([]byte, error) {
		data, ok := f.files[strings.TrimPrefix(address, f.service.BaseURL)]
		if !ok {
			return nil, fs.ErrNotExist
		}
		if int64(len(data)) > limit {
			return nil, fs.ErrInvalid
		}
		return data, nil
	}
	f.release(t, "2.0.0", "stable", archive(t, []byte("new executable")))
	return f
}

func archive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tarball := tar.NewWriter(gz)
	must(t, tarball.WriteHeader(&tar.Header{Name: "kitbag", Mode: 0o755, Size: int64(len(binary))}))
	_, err := tarball.Write(binary)
	must(t, err)
	must(t, tarball.Close())
	must(t, gz.Close())
	return out.Bytes()
}

func (f *fixture) release(t *testing.T, version, channel string, data []byte) {
	t.Helper()
	base := "/latest/download/"
	if channel == "nightly" {
		base = "/download/nightly/"
	}
	name := "kitbag_" + version + "_linux_amd64.tar.gz"
	f.files[base+"kitbag-update.json"] = []byte(fmt.Sprintf(`{"version":%q,"channel":%q}`, version, channel))
	checksums := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name))
	digest := sha256.Sum256(checksums)
	signature, err := ecdsa.SignASN1(rand.Reader, f.key, digest[:])
	must(t, err)
	f.files[base+"checksums.txt"] = checksums
	f.files[base+"checksums.txt.sig"] = []byte(base64.StdEncoding.EncodeToString(signature))
	f.files[base+name] = data
}

func (f *fixture) assertBinary(t *testing.T, want string) {
	t.Helper()
	data, err := f.host.ReadFile(f.service.Binary)
	must(t, err)
	if string(data) != want {
		t.Fatalf("binary: %q; want %q", data, want)
	}
}

func TestVerifiedUpdate(t *testing.T) {
	t.Parallel()
	f := setup(t)
	result, err := f.service.Run(t.Context(), Options{Channel: "stable"})
	must(t, err)
	if !result.Updated || result.Version != "2.0.0" || f.execs != 1 {
		t.Fatalf("result: %+v, execs %d", result, f.execs)
	}
	f.assertBinary(t, "new executable")
	if _, err = f.host.Lstat(f.service.Binary + ".old"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup not cleaned: %v", err)
	}
}

func TestRejectedDownloads(t *testing.T) {
	t.Parallel()
	for name, mutate := range rejectedDownloads() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			mutate(t, f)
			if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
				t.Fatal("accepted unverified release")
			}
			f.assertBinary(t, "old executable")
			if f.execs != 0 {
				t.Fatal("executed rejected release")
			}
			entries, err := f.host.ReadDir(filepath.Dir(f.service.Binary))
			must(t, err)
			if len(entries) != 2 {
				t.Fatalf("failure wrote files: %v", entries)
			}
		})
	}
}

func rejectedDownloads() map[string]func(*testing.T, *fixture) {
	const base = "/latest/download/"
	return map[string]func(*testing.T, *fixture){
		"tampered archive": func(t *testing.T, f *fixture) {
			t.Helper()
			f.files[base+"kitbag_2.0.0_linux_amd64.tar.gz"] = archive(t, []byte("malicious executable"))
		},
		"bad signature": func(_ *testing.T, f *fixture) {
			f.files[base+"checksums.txt.sig"] = []byte(base64.StdEncoding.EncodeToString([]byte("bad signature")))
		},
		"truncated": func(t *testing.T, f *fixture) {
			t.Helper()
			data := archive(t, []byte("new"))
			f.release(t, "2.0.0", "stable", data[:len(data)-1])
		},
		"wrong channel": func(_ *testing.T, f *fixture) {
			f.files[base+"kitbag-update.json"] = []byte(`{"version":"2.0.0-nightly.1","channel":"nightly"}`)
		},
		"bad metadata": func(_ *testing.T, f *fixture) { f.files[base+"kitbag-update.json"] = []byte(`{`) },
		"invalid version": func(_ *testing.T, f *fixture) {
			f.files[base+"kitbag-update.json"] = []byte(`{"version":"../bad","channel":"stable"}`)
		},
		"missing archive":   func(_ *testing.T, f *fixture) { delete(f.files, base+"kitbag_2.0.0_linux_amd64.tar.gz") },
		"missing signature": func(_ *testing.T, f *fixture) { delete(f.files, base+"checksums.txt.sig") },
		"missing checksums": func(_ *testing.T, f *fixture) { delete(f.files, base+"checksums.txt") },
		"missing metadata":  func(_ *testing.T, f *fixture) { delete(f.files, base+"kitbag-update.json") },
	}
}

func TestUpdateModesAndChannels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, current, channel, next string
		check, fail                  bool
	}{
		{name: "check", current: "1.0.0", next: "2.0.0", channel: "stable", check: true},
		{name: "same", current: "2.0.0", next: "2.0.0", channel: "stable"},
		{name: "downgrade", current: "3.0.0", next: "2.0.0", channel: "stable", fail: true},
		{name: "invalid current", current: "dev", next: "2.0.0", channel: "stable", fail: true},
		{name: "nightly", current: "1.0.0", next: "2.0.0-nightly.1", channel: "nightly", check: true},
		{name: "stable in nightly", current: "1.0.0", next: "2.0.0", channel: "nightly", check: true, fail: true},
		{name: "unknown channel", current: "1.0.0", next: "2.0.0", channel: "beta", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			f.service.Current = tc.current
			f.release(t, tc.next, tc.channel, archive(t, []byte("new")))
			result, err := f.service.Run(t.Context(), Options{Channel: tc.channel, Check: tc.check})
			if (err != nil) != tc.fail || result.Updated || f.execs != 0 {
				t.Fatalf("result %+v, error %v", result, err)
			}
			f.assertBinary(t, "old executable")
		})
	}
}

func TestInstallSources(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"homebrew", "deb", "rpm", "aur", "go-install", "missing", "unrecognized"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			stamp := filepath.Join(filepath.Dir(f.service.Binary), "kitbag.install-source")
			must(t, f.host.Remove(stamp))
			if source != "missing" {
				must(t, f.host.WriteAtomic(stamp, []byte(source), binaryPerm))
			}
			f.service.Fetch = func(context.Context, string, int64) ([]byte, error) {
				t.Error("fetched package-owned release")
				return nil, fs.ErrPermission
			}
			result, err := f.service.Run(t.Context(), Options{Channel: "stable"})
			must(t, err)
			if result.Command == "" || result.Updated || f.execs != 0 {
				t.Fatalf("result %+v", result)
			}
			f.assertBinary(t, "old executable")
		})
	}
}

func TestReplacementFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []fake.WriteStage{fake.PartialWrite, fake.BeforeRename} {
		t.Run(fmt.Sprint(stage), func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			must(t, f.host.InterruptNextWrite(f.service.Binary, stage))
			if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
				t.Fatal("replacement succeeded")
			}
			f.assertBinary(t, "old executable")
			if f.execs != 0 {
				t.Fatal("executed after failure")
			}
		})
	}
}

func TestConfirmationRollback(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{false, true} {
		t.Run(strconv.FormatBool(failure), func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			f.service.Probe = func(context.Context, string) (string, error) {
				data, err := f.host.ReadFile(f.service.Binary + ".old")
				if errors.Is(err, fs.ErrNotExist) {
					return "kitbag 1.0.0", nil
				}
				must(t, err)
				if string(data) != "old executable" {
					t.Fatal("backup lost before confirmation")
				}
				if failure {
					return "", fs.ErrPermission
				}
				return "kitbag wrong-version", nil
			}
			if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
				t.Fatal("confirmation accepted")
			}
			f.assertBinary(t, "old executable")
			if f.execs != 0 {
				t.Fatal("re-exec on failure")
			}
		})
	}
}
