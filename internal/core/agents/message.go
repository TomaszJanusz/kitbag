package agents

import (
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
)

// maxTitle is the longest generated commit title; longer titles drop the
// directory, which the body and diff still show.
const maxTitle = 72

// message renders the Conventional Commit title and the full message with a
// body for additional edits and the Kitbag-Host and Kitbag-Op trailers. It
// uses only names, kinds and ~/ paths from the change, never file contents.
func message(c Change, edits []manifestedit.Edit, host string) (string, string) {
	subject := subjectOf(c, edits)
	title := "chore(kitbag): " + subject
	short := len(title) > maxTitle
	if short {
		title = "chore(kitbag): " + string(c.Op) + " " + c.Kind + " instance " + c.Name
	}
	var body []string
	for _, e := range edits {
		if e.Op == manifestedit.AddInstance && e.Name != c.Name {
			body = append(body, "- declare the default directory "+e.Dir+" as instance "+e.Name+" so it stays managed")
		}
	}
	if short || len(body) > 0 {
		body = append([]string{"- " + subject}, body...)
	}
	parts := []string{title}
	if len(body) > 0 {
		parts = append(parts, strings.Join(body, "\n"))
	}
	parts = append(parts, "Kitbag-Host: "+host+"\nKitbag-Op: agent."+string(c.Op))
	return title, strings.Join(parts, "\n\n") + "\n"
}

// subjectOf describes the change: kind, instance, target when it differs
// from the kind, and the new directory or edited values.
func subjectOf(c Change, edits []manifestedit.Edit) string {
	subject := string(c.Op) + " " + c.Kind + " instance " + c.Name
	if c.Target != c.Kind {
		subject += " of target " + c.Target
	}
	switch c.Op {
	case Add:
		subject += " at " + c.Dir
		if len(c.Hosts) > 0 {
			subject += " (hosts " + strings.Join(c.Hosts, ", ") + ")"
		}
	case Edit:
		details := make([]string, 0, len(edits))
		for _, e := range edits {
			details = append(details, editDetails(e)...)
		}
		subject += " (" + strings.Join(details, ", ") + ")"
	case Remove:
	}
	return subject
}

func editDetails(e manifestedit.Edit) []string {
	if e.Op == manifestedit.SetInstanceHosts {
		if len(e.Hosts) == 0 {
			return []string{"all hosts"}
		}
		return []string{"hosts " + strings.Join(e.Hosts, ", ")}
	}
	var details []string
	if e.Dir != "" {
		details = append(details, "dir "+e.Dir)
	}
	if e.Settings != nil {
		state := "off"
		if *e.Settings {
			state = "on"
		}
		details = append(details, "settings "+state)
	}
	return details
}
