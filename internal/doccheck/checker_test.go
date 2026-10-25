package doccheck

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseDocument(t *testing.T) {
	t.Parallel()
	data := "# **Hello**, `world`! &amp; café\n\n" +
		"# Repeat\n# Repeat\n# Repeat-1\n# Repeat\n\n" +
		"Setext heading\n--------------\n\n" +
		"<a id=\"custom\"></a> <a name='legacy'></a> <a data-id='not-id'></a>\n\n" +
		"[inline](other.md#part \"title\") [reference][target] [target][]\n\n" +
		"[target]: other.md#part\n\n" +
		"![image](picture.png) [escaped](dir/file_\\(one\\).md)\n\n" +
		"`[code](missing.md)`\n\n```md\n# Fake heading\n[code](missing.md)\n```\n\n" +
		"    [indented](missing.md)\n\n<!-- <a id=\"comment\"></a> -->\n"
	got := parseDocument([]byte(data))
	want := []string{"other.md#part", "other.md#part", "other.md#part", "picture.png", "dir/file_(one).md"}
	if !reflect.DeepEqual(got.links, want) {
		t.Errorf("links = %q, want %q", got.links, want)
	}
	for _, anchor := range []string{"hello-world--café", "repeat", "repeat-1", "repeat-1-1", "repeat-2", "setext-heading", "custom", "legacy"} {
		if !got.anchors[anchor] {
			t.Errorf("missing anchor %q", anchor)
		}
	}
	for _, anchor := range []string{"fake-heading", "comment", "not-id"} {
		if got.anchors[anchor] {
			t.Errorf("unexpected anchor %q", anchor)
		}
	}
}

func TestCheckLink(t *testing.T) {
	t.Parallel()
	root := fstest.MapFS{
		"docs/source.md":     {Data: []byte("# Source\n")},
		"docs/target.md":     {Data: []byte("# Part\n\n<a id='explicit'></a>\n")},
		"docs/a b.md":        {Data: []byte("# Héllo\n")},
		"README.md":          {Data: []byte("# Root\n")},
		"docs/dir/README.md": {Data: []byte("# Directory\n")},
	}
	tests := []struct{ link, want string }{
		{"target.md#part", ""},
		{"target.md#explicit", ""},
		{"#source", ""},
		{"../README.md#root", ""},
		{"a%20b.md#h%C3%A9llo", ""},
		{"dir", ""},
		{"dir#directory", ""},
		{"target.md?raw=1#part", ""},
		{"https://invalid.example/missing#anchor", ""},
		{"//invalid.example/missing", ""},
		{"mailto:someone@example.com", ""},
		{"/absolute/site/path", ""},
		{"missing.md", "does not exist"},
		{"target.md#missing", "missing anchor #missing"},
		{"#missing", "missing anchor #missing"},
		{"target.md#Part", "missing anchor #Part"},
		{"%zz", "invalid URL escape"},
	}
	for _, test := range tests {
		t.Run(test.link, func(t *testing.T) {
			t.Parallel()
			err := checkLink(root, "docs/source.md", test.link)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %v, want %q", err, test.want)
			}
		})
	}
}
