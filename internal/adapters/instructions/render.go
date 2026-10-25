package instructions

import "bytes"

// Marker is the first line of every generated instructions file. It replaces
// agent-config-sync's repository-specific marker; it is the same on every
// host, so rendering stays independent of where the working copy lives.
const Marker = "<!-- Managed by kitbag. Edit the source in the configuration repository, not this file. -->"

// Managed reports whether content starts with Marker, as Import and Concat
// output does. Ownership still comes from local state, never from the marker.
func Managed(content []byte) bool {
	return bytes.HasPrefix(content, []byte(Marker+"\n"))
}

// renderImport reproduces agent-config-sync's CLAUDE.md: the marker, a blank
// line, then one "@<absolute path>" line per input.
func renderImport(paths []string) []byte {
	out := []byte(Marker + "\n\n")
	for _, path := range paths {
		out = append(out, "@"+path+"\n"...)
	}
	return out
}

// renderConcat reproduces agent-config-sync's AGENTS.md: the marker and each
// input separated by a blank line, with a single final newline. Like the shell's
// "$(cat file)", it drops every trailing newline of each input first.
func renderConcat(parts [][]byte) []byte {
	out := []byte(Marker)
	for _, part := range parts {
		out = append(out, "\n\n"...)
		out = append(out, bytes.TrimRight(part, "\n")...)
	}
	return append(out, '\n')
}
