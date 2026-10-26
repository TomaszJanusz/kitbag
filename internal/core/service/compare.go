package service

import (
	"bytes"
	"encoding/xml"
	"reflect"
	"slices"
	"strings"
)

// Ignore comments and presentation whitespace, never argument/string contents.
// Dictionary order is insignificant in a plist; array and unit directive order
// remain significant (systemd permits repeated directives).
func equivalent(manager string, a, b []byte) bool {
	if manager != "launchd" {
		return slices.Equal(unitLines(a), unitLines(b))
	}
	var left, right plistNode
	if xml.Unmarshal(a, &left) != nil || xml.Unmarshal(b, &right) != nil {
		return bytes.Equal(a, b)
	}
	left.normalize()
	right.normalize()
	return reflect.DeepEqual(left, right)
}

func unitLines(data []byte) []string {
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, ";") {
			lines = append(lines, line)
		}
	}
	return lines
}

const dictPairSize = 2

type plistNode struct {
	XMLName  xml.Name
	Text     string      `xml:",chardata"`
	Children []plistNode `xml:",any"`
}

func (n *plistNode) normalize() {
	if len(n.Children) > 0 {
		n.Text = ""
	}
	for i := range n.Children {
		n.Children[i].normalize()
	}
	if n.XMLName.Local != "dict" || len(n.Children)%2 != 0 {
		return
	}
	type pair struct{ key, value plistNode }
	pairs := make([]pair, 0, len(n.Children)/dictPairSize)
	for i := 0; i < len(n.Children); i += 2 {
		pairs = append(pairs, pair{n.Children[i], n.Children[i+1]})
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return strings.Compare(a.key.Text, b.key.Text) })
	for i, p := range pairs {
		n.Children[2*i], n.Children[2*i+1] = p.key, p.value
	}
}
