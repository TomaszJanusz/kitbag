package system

import (
	"errors"
	"testing"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		out  string
		want Version
	}{
		{"git version 2.43.0\n", Version{"2.43.0", 2, 43, 0}},
		{"git version 2.39.3 (Apple Git-145)\n", Version{"2.39.3 (Apple Git-145)", 2, 39, 3}},
		{"git version 2.45.1.windows.1\n", Version{"2.45.1.windows.1", 2, 45, 1}},
		{"git version 2.30.0.rc1", Version{"2.30.0.rc1", 2, 30, 0}},
		{"git version 2.30\n", Version{"2.30", 2, 30, 0}},
		{"  git version 3.0.0  \nextra line\n", Version{"3.0.0", 3, 0, 0}},
		{"git version 2.34.1.vfs.0.0", Version{"2.34.1.vfs.0.0", 2, 34, 1}},
	} {
		got, err := ParseVersion(tt.out)
		if err != nil || got != tt.want {
			t.Errorf("ParseVersion(%q) = %+v, %v; want %+v", tt.out, got, err, tt.want)
		}
	}
	for _, out := range []string{"", "git version", "hub version 2.14", "git version x.y", "version 2.40.0", "git version 99999999999999999999.1"} {
		if _, err := ParseVersion(out); !errors.Is(err, ErrUnknownVersion) {
			t.Errorf("ParseVersion(%q) error = %v, want ErrUnknownVersion", out, err)
		}
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		v  Version
		ok bool
	}{
		{Version{"", 2, 30, 0}, true},
		{Version{"", 2, 30, 9}, true},
		{Version{"", 2, 43, 0}, true},
		{Version{"", 3, 0, 0}, true},
		{Version{"", 2, 29, 99}, false},
		{Version{"", 1, 99, 0}, false},
	} {
		err := checkVersion(tt.v)
		if (err == nil) != tt.ok {
			t.Errorf("checkVersion(%d.%d.%d) = %v, want ok=%v", tt.v.Major, tt.v.Minor, tt.v.Patch, err, tt.ok)
		}
		var vErr *VersionError
		if err != nil && (!errors.Is(err, ErrGitTooOld) || !errors.As(err, &vErr)) {
			t.Errorf("checkVersion error %v is not a *VersionError matching ErrGitTooOld", err)
		}
	}
	v, err := ParseVersion("git version 2.29.2")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := checkVersion(v).Error(), "git 2.29.2 is older than the required 2.30"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}
