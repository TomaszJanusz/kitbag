package manifest

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CheckName validates a target, instance, server or import name with the same
// grammar Parse applies to those table keys.
func CheckName(name string) error {
	return checkGrammar(name, namePattern, "use up to 64 lowercase letters, digits, dots, underscores or hyphens, starting with a letter or digit")
}

// CheckHostName validates a host name with the same grammar Parse applies to
// [hosts] keys and files[].hosts entries.
func CheckHostName(name string) error {
	return checkGrammar(name, hostPattern, "use up to 64 letters, digits, dots, underscores or hyphens, starting with a letter or digit")
}

// CheckHomePath validates a home-relative destination ("~/..." with portable,
// non-dot segments), such as an instance dir, as Parse does.
func CheckHomePath(path string) error {
	if !matches(pathRule("home"), path) || utf8.RuneCountInString(path) > maxPathLength {
		return errors.New("use a path below the home directory that starts with ~/ and has no . or .. segments")
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "~/"), "/") {
		if err := portableName(part); err != nil {
			return err
		}
	}
	return nil
}

func checkGrammar(name, pattern, hint string) error {
	if !matches(textRule(pattern), name) {
		return errors.New("invalid name " + strconv.Quote(name) + ": " + hint)
	}
	return portableName(name)
}
