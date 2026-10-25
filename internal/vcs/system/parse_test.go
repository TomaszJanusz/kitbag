package system

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// nul joins records the way -z output terminates them.
func nul(records ...string) []byte {
	return []byte(strings.Join(records, "\x00") + "\x00")
}

const (
	oid  = "e1b47a058bb15b1d4e38e3feabfc93a0cb9f7eac"
	oid2 = "45b983be36b73c0788dc9cbcb76cbb80fc7bb057"
)

func TestParseStatus(t *testing.T) {
	t.Parallel()

	out := nul(
		"# branch.oid "+oid,
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +1 -2",
		"# stash 3",
		"1 .M N... 100644 100644 100644 "+oid+" "+oid+" dir with space/a b.txt",
		"1 A. N... 000000 100644 100644 "+strings.Repeat("0", 40)+" "+oid2+" new\nline",
		"1 D. N... 100644 000000 000000 "+oid+" "+strings.Repeat("0", 40)+" gone",
		"1 .T N... 100644 120000 120000 "+oid+" "+oid+" now-a-link",
		"2 R. N... 100644 100644 100644 "+oid+" "+oid+" R100 new name",
		"old name",
		"2 C. N... 100644 100644 100644 "+oid+" "+oid+" C75 copy",
		"orig",
		"u UU N... 100644 100644 100644 100644 "+oid+" "+oid+" "+oid+" conflict file",
		"? ünï ✓",
		"! ignored",
	)
	got, err := parseStatus(out)
	if err != nil {
		t.Fatal(err)
	}
	want := vcs.Status{
		Head:     vcs.Head{Branch: "main", Commit: oid},
		Upstream: "origin/main", Ahead: 1, Behind: 2,
		Changes: []vcs.Change{
			{Path: "dir with space/a b.txt", Kind: vcs.ChangeModified},
			{Path: "new\nline", Kind: vcs.ChangeAdded},
			{Path: "gone", Kind: vcs.ChangeDeleted},
			{Path: "now-a-link", Kind: vcs.ChangeTypeChanged},
			{Path: "new name", OrigPath: "old name", Kind: vcs.ChangeRenamed},
			{Path: "copy", OrigPath: "orig", Kind: vcs.ChangeCopied},
			{Path: "conflict file", Kind: vcs.ChangeUnmerged},
			{Path: "ünï ✓", Kind: vcs.ChangeUntracked},
		},
	}
	if got.Head != want.Head || got.Upstream != want.Upstream || got.Ahead != 1 || got.Behind != 2 || got.UpstreamGone {
		t.Errorf("header = %+v, want %+v", got, want)
	}
	if !slices.Equal(got.Changes, want.Changes) {
		t.Errorf("changes = %q\nwant      %q", got.Changes, want.Changes)
	}
}

func TestParseStatusBranchStates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		out  []byte
		want vcs.Status
	}{
		{"empty output", nil, vcs.Status{}},
		{"unborn", nul("# branch.oid (initial)", "# branch.head main"), vcs.Status{Head: vcs.Head{Branch: "main"}}},
		{"detached", nul("# branch.oid "+oid, "# branch.head (detached)"), vcs.Status{Head: vcs.Head{Commit: oid}}},
		{
			"upstream gone", nul("# branch.oid "+oid, "# branch.head main", "# branch.upstream origin/old"),
			vcs.Status{Head: vcs.Head{Branch: "main", Commit: oid}, Upstream: "origin/old", UpstreamGone: true},
		},
		{
			"unborn upstream is left to the caller", nul("# branch.oid (initial)", "# branch.head main", "# branch.upstream origin/main"),
			vcs.Status{Head: vcs.Head{Branch: "main"}, Upstream: "origin/main"},
		},
	} {
		got, err := parseStatus(tt.out)
		if err != nil || got.Head != tt.want.Head || got.Upstream != tt.want.Upstream ||
			got.UpstreamGone != tt.want.UpstreamGone || !got.Clean() {
			t.Errorf("%s: parseStatus = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}

func TestParseStatusRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, out := range [][]byte{
		nul("# branch.ab +x -1"),
		nul("# branch.ab +1"),
		nul("1 .M N... 100644 100644 100644 " + oid + " " + oid),
		nul("1 M N... 100644 100644 100644 " + oid + " " + oid + " path"),
		nul("1 .Z N... 100644 100644 100644 " + oid + " " + oid + " path"),
		nul("2 R. N... 100644 100644 100644 " + oid + " " + oid + " R100 lonely"),
		nul("x unknown"),
		[]byte("\x00\x00"),
	} {
		if s, err := parseStatus(out); !errors.Is(err, ErrUnexpectedOutput) {
			t.Errorf("parseStatus(%q) = %+v, %v; want ErrUnexpectedOutput", out, s, err)
		}
	}
}

func TestParseDiff(t *testing.T) {
	t.Parallel()

	got, err := parseDiff(nul("A", "a b", "M", "new\nline", "D", "ünï", "T", "link", "U", "conflict"))
	want := []vcs.Change{
		{Path: "a b", Kind: vcs.ChangeAdded},
		{Path: "new\nline", Kind: vcs.ChangeModified},
		{Path: "ünï", Kind: vcs.ChangeDeleted},
		{Path: "link", Kind: vcs.ChangeTypeChanged},
		{Path: "conflict", Kind: vcs.ChangeUnmerged},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("parseDiff = %q, %v; want %q", got, err, want)
	}
	if got, err := parseDiff(nil); err != nil || len(got) != 0 {
		t.Errorf("parseDiff(empty) = %q, %v", got, err)
	}
	for _, out := range [][]byte{nul("M"), nul("R100", "a", "b"), nul("X", "a"), nul("MM", "a"), nul("M", "")} {
		if _, err := parseDiff(out); !errors.Is(err, ErrUnexpectedOutput) {
			t.Errorf("parseDiff(%q) error = %v, want ErrUnexpectedOutput", out, err)
		}
	}
}

func TestPushRejection(t *testing.T) {
	t.Parallel()

	rejected := "To /srv/r.git\n!\trefs/heads/main:refs/heads/main\t[rejected] (fetch first)\nDone\n"
	if reason, ok := pushRejection([]byte(rejected)); !ok || !strings.Contains(reason, "[rejected] (fetch first)") {
		t.Errorf("pushRejection(rejected) = %q, %v", reason, ok)
	}
	for _, out := range []string{
		"To /srv/r.git\n \trefs/heads/main:refs/heads/main\te1b47a0..45b983b\nDone\n",
		"To /srv/r.git\n=\trefs/heads/main:refs/heads/main\t[up to date]\nDone\n",
		"",
	} {
		if reason, ok := pushRejection([]byte(out)); ok {
			t.Errorf("pushRejection(%q) = %q, want accepted", out, reason)
		}
	}
}
