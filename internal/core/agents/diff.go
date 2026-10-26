package agents

import (
	"fmt"
	"strings"
)

// context is the number of unchanged lines shown around each change.
const context = 2

// Diff returns a unified diff of two manifests, one line per entry without
// newline characters. It uses a longest-common-subsequence table, which is
// ample for manifest sizes.
func Diff(before, after []byte) []string {
	a, b := lines(before), lines(after)
	ops := script(a, b)
	result := []string{}
	for start := 0; start < len(ops); {
		first := next(ops, start)
		if first < 0 {
			break
		}
		from, to := max(first-context, 0), hunkEnd(ops, first)
		result = append(result, hunk(ops[from:to])...)
		start = to
	}
	return result
}

type edit struct {
	text         string
	kind         byte // ' ', '-' or '+'
	oldAt, newAt int  // one-based line numbers before the entry
}

func lines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func script(a, b []string) []edit {
	lcs := table(a, b)
	var ops []edit
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops = append(ops, edit{a[i], ' ', i, j})
			i, j = i+1, j+1
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			ops = append(ops, edit{b[j], '+', i, j})
			j++
		default:
			ops = append(ops, edit{a[i], '-', i, j})
			i++
		}
	}
	return ops
}

// table returns the longest common subsequence lengths of every suffix pair.
func table(a, b []string) [][]int {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	return lcs
}

func next(ops []edit, from int) int {
	for i := from; i < len(ops); i++ {
		if ops[i].kind != ' ' {
			return i
		}
	}
	return -1
}

// hunkEnd extends a hunk over changes separated by at most 2*context
// unchanged lines and returns its exclusive end.
func hunkEnd(ops []edit, first int) int {
	end, unchanged := first, 0
	for i := first; i < len(ops) && unchanged <= 2*context; i++ {
		if ops[i].kind == ' ' {
			unchanged++
			continue
		}
		end, unchanged = i, 0
	}
	return min(end+1+context, len(ops))
}

func hunk(ops []edit) []string {
	oldLen, newLen := 0, 0
	body := make([]string, 0, len(ops))
	for _, op := range ops {
		if op.kind != '+' {
			oldLen++
		}
		if op.kind != '-' {
			newLen++
		}
		body = append(body, string(op.kind)+op.text)
	}
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", start(ops[0].oldAt, oldLen), oldLen, start(ops[0].newAt, newLen), newLen)
	return append([]string{header}, body...)
}

// start is a hunk side's first line number; an empty side names the line
// before it, as unified diffs do.
func start(at, length int) int {
	if length == 0 {
		return at
	}
	return at + 1
}
