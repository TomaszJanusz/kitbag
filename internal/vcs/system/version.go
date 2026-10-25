package system

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// The minimum supported Git version, 2.30 (ADR 4).
const (
	minMajor = 2
	minMinor = 30
)

// Errors from [New].
var (
	// ErrGitNotFound reports that no git executable is on PATH.
	ErrGitNotFound = vcs.ErrSystemGitNotFound
	// ErrGitTooOld reports a Git older than 2.30. It is wrapped in a
	// [*VersionError].
	ErrGitTooOld = errors.New("git is too old")
	// ErrUnknownVersion reports "git --version" output that is not a Git
	// version.
	ErrUnknownVersion = errors.New("unrecognized git version")
)

// versionPattern matches "git version 2.39.3 (Apple Git-145)",
// "git version 2.45.1.windows.1", "git version 2.30.0.rc1" and plain versions.
var versionPattern = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

// Version is a Git release.
type Version struct {
	// Raw is the version as Git printed it, including any vendor suffix.
	Raw                 string
	Major, Minor, Patch int
}

// MinVersion returns the oldest supported Git release.
func MinVersion() Version {
	return Version{Major: minMajor, Minor: minMinor, Raw: fmt.Sprintf("%d.%d", minMajor, minMinor)}
}

// String returns the raw version.
func (v Version) String() string {
	return v.Raw
}

// Less reports whether v is an older release than w.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

// ParseVersion parses the first line of "git --version".
func ParseVersion(output string) (Version, error) {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	line = strings.TrimSpace(line)
	m := versionPattern.FindStringSubmatch(line)
	if m == nil {
		return Version{}, fmt.Errorf("%w: %q", ErrUnknownVersion, line)
	}
	var nums [3]int
	for i, s := range m[1:] {
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return Version{}, fmt.Errorf("%w: %q: %w", ErrUnknownVersion, line, err)
		}
		nums[i] = n
	}
	return Version{
		Major: nums[0], Minor: nums[1], Patch: nums[2],
		Raw: strings.TrimPrefix(line, "git version "),
	}, nil
}

// VersionError reports an unsupported Git version. It matches [ErrGitTooOld].
type VersionError struct {
	Found, Min Version
}

// Error names both versions.
func (e *VersionError) Error() string {
	return fmt.Sprintf("git %s is older than the required %s", e.Found, e.Min)
}

// Unwrap returns [ErrGitTooOld].
func (*VersionError) Unwrap() error {
	return ErrGitTooOld
}

// checkVersion enforces [MinVersion].
func checkVersion(v Version) error {
	if v.Less(MinVersion()) {
		return &VersionError{Found: v, Min: MinVersion()}
	}
	return nil
}
