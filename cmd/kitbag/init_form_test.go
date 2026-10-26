package main

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// Drain the real form's navigation commands synchronously. Cursor animation and
// terminal queries are deliberately excluded: this drives the huh model with
// key messages, without starting a program, reading a TTY or using timers.
func formKey(t *testing.T, form *huh.Form, key tea.KeyMsg) {
	t.Helper()
	_, cmd := form.Update(key)
	drainForm(t, form, cmd, 0)
}

func drainForm(t *testing.T, form *huh.Form, cmd tea.Cmd, depth int) {
	t.Helper()
	const maxDepth = 50
	if cmd == nil {
		return
	}
	if depth > maxDepth {
		t.Fatal("form command loop")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			drainForm(t, form, child, depth+1)
		}
		return
	}
	if msg == nil || reflect.TypeOf(msg).PkgPath() != "github.com/charmbracelet/huh" {
		return
	}
	_, next := form.Update(msg)
	drainForm(t, form, next, depth+1)
}

func TestInitFormKeyboardChoices(t *testing.T) {
	t.Parallel()
	flags := initFlags{backend: "auto", mode: "link"}
	form := initForm(&flags, false, false)
	_ = form.GetFocusedField().Focus()
	formKey(t, form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/config.git")})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyDown})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyDown})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyDown})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	if form.State != huh.StateCompleted || flags.repo != "https://example.com/config.git" || flags.backend != "embedded" || flags.mode != "copy" {
		t.Fatalf("state=%v choices=%+v", form.State, flags)
	}
}

func TestInitFormValidationAndCancellation(t *testing.T) {
	t.Parallel()
	flags := initFlags{backend: "auto", mode: "link"}
	form := initForm(&flags, false, false)
	_ = form.GetFocusedField().Focus()
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	if form.State != huh.StateNormal || form.GetFocusedField().Error() == nil {
		t.Fatal("empty repository accepted")
	}
	formKey(t, form, tea.KeyMsg{Type: tea.KeyCtrlC})
	if form.State != huh.StateAborted {
		t.Fatal("cancel did not abort")
	}
}

func TestInitConfirmationKeyboard(t *testing.T) {
	t.Parallel()
	for _, answer := range []bool{false, true} {
		t.Run(map[bool]string{false: "decline", true: "accept"}[answer], func(t *testing.T) {
			t.Parallel()
			confirmed := false
			form := confirmationForm(&confirmed, "Save?")
			_ = form.GetFocusedField().Focus()
			key := 'n'
			if answer {
				key = 'y'
			}
			formKey(t, form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
			formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
			if form.State != huh.StateCompleted || confirmed != answer {
				t.Fatalf("confirmation=%t state=%v", confirmed, form.State)
			}
		})
	}
}

func TestInitIdentityKeyboard(t *testing.T) {
	t.Parallel()
	var flags initFlags
	form := identityForm(&flags)
	_ = form.GetFocusedField().Focus()
	formKey(t, form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Publisher")})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("publisher@example.com")})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	if form.State != huh.StateCompleted || flags.name != "Publisher" || flags.email != "publisher@example.com" {
		t.Fatalf("identity=%+v state=%v", flags, form.State)
	}
}

func TestScaffoldFormKeyboard(t *testing.T) {
	t.Parallel()
	flags := initFlags{agents: []string{"claude"}}
	form := scaffoldForm(&flags, false, false)
	_ = form.GetFocusedField().Focus()
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("claude.main=~/.claude")})
	formKey(t, form, tea.KeyMsg{Type: tea.KeyEnter})
	flags.parseInstances()
	if form.State != huh.StateCompleted || len(flags.agents) != 1 || len(flags.instances) != 1 || flags.instances[0] != "claude.main=~/.claude" {
		t.Fatalf("scaffold choices=%+v state=%v", flags, form.State)
	}
}

// The service question defaults to yes: Enter alone installs, n declines.
func TestServiceFormDefaultsToYes(t *testing.T) {
	t.Parallel()
	for name, keys := range map[string][]tea.KeyMsg{
		"enter":   {{Type: tea.KeyEnter}},
		"decline": {{Type: tea.KeyRunes, Runes: []rune{'n'}}, {Type: tea.KeyEnter}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			accepted := false
			form := serviceForm(&accepted)
			_ = form.GetFocusedField().Focus()
			for _, key := range keys {
				formKey(t, form, key)
			}
			if want := name == "enter"; form.State != huh.StateCompleted || accepted != want {
				t.Fatalf("accepted=%t want %t state=%v", accepted, want, form.State)
			}
		})
	}
}
