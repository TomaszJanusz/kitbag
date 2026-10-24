package system

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// ErrUnexpectedOutput reports Git output that does not have the documented
// machine-readable format.
var ErrUnexpectedOutput = errors.New("unexpected git output")

// Field counts before the path in "git status --porcelain=v2" entries.
const (
	ordinaryFields = 8  // 1 XY sub mH mI mW hH hI path
	renameFields   = 9  // 2 XY sub mH mI mW hH hI Xscore path NUL orig
	unmergedFields = 10 // u XY sub m1 m2 m3 mW h1 h2 h3 path
)

// parseStatus parses "git status --porcelain=v2 --branch -z" output, in which
// every header and entry ends with NUL and paths are verbatim.
func parseStatus(out []byte) (vcs.Status, error) {
	var s vcs.Status
	hasAB := false
	records := splitNUL(out)
	for i := 0; i < len(records); i++ {
		rec := records[i]
		var err error
		switch {
		case strings.HasPrefix(rec, "# "):
			hasAB, err = parseHeader(&s, rec, hasAB)
		case strings.HasPrefix(rec, "2 "):
			// The source path of a rename or copy is the next record.
			if i+1 == len(records) {
				return vcs.Status{}, unexpected("status", rec)
			}
			i++
			err = parseEntry(&s, rec, records[i])
		default:
			err = parseEntry(&s, rec, "")
		}
		if err != nil {
			return vcs.Status{}, err
		}
	}
	// A configured upstream without counts has no remote-tracking branch,
	// unless the local branch is unborn (handled by the caller).
	s.UpstreamGone = s.Upstream != "" && !hasAB && !s.Unborn()
	return s, nil
}

func parseHeader(s *vcs.Status, rec string, hasAB bool) (bool, error) {
	key, value, _ := strings.Cut(strings.TrimPrefix(rec, "# "), " ")
	switch key {
	case "branch.oid":
		if value != "(initial)" {
			s.Commit = value
		}
	case "branch.head":
		if value != "(detached)" {
			s.Branch = value
		}
	case "branch.upstream":
		s.Upstream = value
	case "branch.ab":
		ahead, behind, ok := strings.Cut(value, " ")
		a, errA := strconv.Atoi(strings.TrimPrefix(ahead, "+"))
		b, errB := strconv.Atoi(strings.TrimPrefix(behind, "-"))
		if !ok || errA != nil || errB != nil {
			return hasAB, unexpected("status", rec)
		}
		s.Ahead, s.Behind = a, b
		return true, nil
	}
	// Other headers (such as "# stash") carry nothing kitbag needs.
	return hasAB, nil
}

func parseEntry(s *vcs.Status, rec, orig string) error {
	if rec == "" {
		return unexpected("status", rec)
	}
	var (
		c   vcs.Change
		err error
	)
	switch rec[0] {
	case '1':
		c, err = trackedChange(rec, ordinaryFields)
	case '2':
		c, err = trackedChange(rec, renameFields)
		c.OrigPath = orig
	case 'u':
		c, err = trackedChange(rec, unmergedFields)
		c.Kind = vcs.ChangeUnmerged
	case '?':
		c = vcs.Change{Path: strings.TrimPrefix(rec, "? "), Kind: vcs.ChangeUntracked}
	case '!':
		return nil
	default:
		return unexpected("status", rec)
	}
	if err != nil {
		return err
	}
	s.Changes = append(s.Changes, c)
	return nil
}

// trackedChange parses an entry with fields space-separated values before
// the path, which may itself contain spaces.
func trackedChange(rec string, fields int) (vcs.Change, error) {
	parts := strings.SplitN(rec, " ", fields+1)
	if len(parts) != fields+1 || len(parts[1]) != 2 || parts[fields] == "" {
		return vcs.Change{}, unexpected("status", rec)
	}
	// XY: the index status, then the working tree status; "." is unchanged.
	code := parts[1][0]
	if code == '.' {
		code = parts[1][1]
	}
	kind, ok := changeKind(code)
	if !ok && rec[0] != 'u' {
		return vcs.Change{}, unexpected("status", rec)
	}
	return vcs.Change{Path: parts[fields], Kind: kind}, nil
}

// changeKind maps a status letter of git status or git diff --name-status.
func changeKind(code byte) (vcs.ChangeKind, bool) {
	switch code {
	case 'M':
		return vcs.ChangeModified, true
	case 'A':
		return vcs.ChangeAdded, true
	case 'D':
		return vcs.ChangeDeleted, true
	case 'T':
		return vcs.ChangeTypeChanged, true
	case 'R':
		return vcs.ChangeRenamed, true
	case 'C':
		return vcs.ChangeCopied, true
	case 'U':
		return vcs.ChangeUnmerged, true
	}
	return 0, false
}

// parseDiff parses "git diff --name-status -z --no-renames": alternating
// status letters and paths, each ending with NUL.
func parseDiff(out []byte) ([]vcs.Change, error) {
	const statusAndPath = 2
	records := splitNUL(out)
	if len(records)%statusAndPath != 0 {
		return nil, unexpected("diff", string(out))
	}
	changes := make([]vcs.Change, 0, len(records)/statusAndPath)
	for i := 0; i < len(records); i += statusAndPath {
		status, path := records[i], records[i+1]
		if len(status) != 1 || path == "" {
			return nil, unexpected("diff", status)
		}
		kind, ok := changeKind(status[0])
		if !ok || kind == vcs.ChangeRenamed || kind == vcs.ChangeCopied {
			return nil, unexpected("diff", status)
		}
		changes = append(changes, vcs.Change{Path: path, Kind: kind})
	}
	return changes, nil
}

// pushRejection finds a rejected ref in "git push --porcelain" output, whose
// ref lines are "<flag> TAB <from>:<to> TAB <summary> [(<reason>)]"; the
// flag "!" means rejected.
func pushRejection(out []byte) (string, bool) {
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Split(line, "\t")
		const flagRefSummary = 3
		if len(fields) == flagRefSummary && fields[0] == "!" {
			return fields[1] + " " + fields[2], true
		}
	}
	return "", false
}

// splitNUL splits NUL-terminated records.
func splitNUL(out []byte) []string {
	out = bytes.TrimSuffix(out, []byte{0})
	if len(out) == 0 {
		return nil
	}
	return strings.Split(string(out), "\x00")
}

func unexpected(cmd, rec string) error {
	return fmt.Errorf("%w from git %s: %q", ErrUnexpectedOutput, cmd, rec)
}
