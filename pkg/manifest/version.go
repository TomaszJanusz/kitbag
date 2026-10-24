package manifest

import (
	"fmt"
	"strings"
)

// VersionError means the running tool is older than MinToolVersion. The CLI can
// classify it with errors.As and offer an explicit update (planned exit code 4).
type VersionError struct {
	Required string
	Current  string
}

// Error returns the required and running tool versions.
func (e *VersionError) Error() string {
	return fmt.Sprintf("manifest requires kitbag %s; running %s", e.Required, e.Current)
}

// CheckVersion compares semantic versions, ignoring build metadata. Release
// versions outrank prereleases; numeric prerelease identifiers compare numerically.
// An invalid current version is an error, including development version labels.
func (m *Manifest) CheckVersion(current string) error {
	if !matches(textRule(versionPattern), current) {
		return fmt.Errorf("invalid tool version %q", current)
	}
	if m.MinToolVersion == "" {
		return nil
	}
	if compareVersion(current, m.MinToolVersion) < 0 {
		v := &VersionError{Required: m.MinToolVersion, Current: current}
		if m.doc != nil {
			return m.doc.fail("/min_tool_version", v.Error(), v)
		}
		return v
	}
	return nil
}

func compareVersion(a, b string) int {
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	coreA, preA, _ := strings.Cut(a, "-")
	coreB, preB, _ := strings.Cut(b, "-")
	partsA, partsB := strings.Split(coreA, "."), strings.Split(coreB, ".")
	for i := range partsA {
		if cmp := compareNumber(partsA[i], partsB[i]); cmp != 0 {
			return cmp
		}
	}
	if preA == preB {
		return 0
	}
	if preA == "" {
		return 1
	}
	if preB == "" {
		return -1
	}
	return comparePrerelease(strings.Split(preA, "."), strings.Split(preB, "."))
}

func comparePrerelease(a, b []string) int {
	for i := range min(len(a), len(b)) {
		x, y := a[i], b[i]
		numericX, numericY := numeric(x), numeric(y)
		switch {
		case numericX && numericY:
			if cmp := compareNumber(x, y); cmp != 0 {
				return cmp
			}
		case numericX:
			return -1
		case numericY:
			return 1
		default:
			if cmp := strings.Compare(x, y); cmp != 0 {
				return cmp
			}
		}
	}
	return len(a) - len(b)
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareNumber(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}
