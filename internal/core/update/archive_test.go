package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"
)

func TestChecksumParsing(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		data string
		fail bool
	}{
		{digest + "  kitbag.tar.gz", false},
		{strings.ToUpper(digest) + " *kitbag.tar.gz", false},
		{"bad kitbag.tar.gz", true},
		{digest + " other.tar.gz", true},
		{digest + " kitbag.tar.gz\n" + digest + " kitbag.tar.gz", true},
	} {
		t.Run(tc.data, func(t *testing.T) {
			t.Parallel()
			got, err := checksum([]byte(tc.data), "kitbag.tar.gz")
			if (err != nil) != tc.fail {
				t.Fatalf("checksum %q: %v", got, err)
			}
		})
	}
}

func TestArchiveSafety(t *testing.T) {
	t.Parallel()
	for _, header := range []tar.Header{
		{Name: "../kitbag", Typeflag: tar.TypeReg, Size: 1},
		{Name: "kitbag", Typeflag: tar.TypeSymlink, Linkname: "outside"},
		{Name: "kitbag", Typeflag: tar.TypeReg, Size: 0},
	} {
		t.Run(fmt.Sprint(header), func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			gz := gzip.NewWriter(&out)
			tw := tar.NewWriter(gz)
			must(t, tw.WriteHeader(&header))
			if header.Size > 0 {
				_, err := tw.Write([]byte("x"))
				must(t, err)
			}
			must(t, tw.Close())
			must(t, gz.Close())
			if _, err := unpack(out.Bytes()); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
	if _, err := unpack([]byte("not gzip")); err == nil {
		t.Fatal("accepted invalid gzip")
	}
	if validEntry(&tar.Header{Name: "kitbag", Typeflag: tar.TypeReg, Size: binaryLimit + 1}, nil) {
		t.Fatal("oversized executable accepted")
	}
}
