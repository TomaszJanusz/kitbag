package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func (s *session) settingRow(index int) {
	s.press("3")
	for range 20 {
		s.press(keyUp)
	}
	for range index {
		s.press(keyDown)
	}
}

func (s *session) noWrite() {
	s.t.Helper()
	select {
	case got := <-s.client.writes:
		s.t.Fatalf("unexpected write: %v", got)
	default:
	}
}

func TestSettingsToggleSavesAndCancels(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.settingRow(6)
	s.press(keyEnter, keySpace)
	s.waitFor(" On [Off]")
	s.press(keyEnter)
	if got := <-s.client.writes; got != [2]string{"daemon.update_check", "false"} {
		t.Fatalf("toggle write=%v", got)
	}
	s.waitFor("daemon.update_check  false · config.toml")
	s.press(keyEnter, "arbitrary", keySpace, keyEscape)
	screen := s.final()
	if !strings.Contains(screen, "daemon.update_check  false · config.toml") || strings.Contains(screen, "Editing:") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsAdvancedAndHiddenKeys(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press("3", keyAdvanced)
	s.settingRow(9)
	s.waitFor(">   git.backend", "Advanced [-]")
	s.press(keyEnter, keySpace, keyEnter)
	if got := <-s.client.writes; got != [2]string{"git.backend", "system"} {
		t.Fatalf("backend write=%v", got)
	}
	s.waitFor("git.backend  system · config.toml")
	golden(t, "settings-advanced", s.final())
}

func TestSettingsCollapseHidesAdvancedAndInternalKeys(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press("3", keyAdvanced, keyAdvanced)
	screen := s.final()
	for _, key := range []string{"schema", "daemon.release_url", "ui.notifications", "ui.theme", "migration.legacy_roots", "repo.branch", "git.backend", "daemon.debounce", "ui.color"} {
		if strings.Contains(screen, key) {
			t.Fatalf("hidden key %s on screen:\n%s", key, screen)
		}
	}
	if !strings.Contains(screen, "Advanced [+]") || !strings.Contains(screen, "sync.mode") {
		t.Fatal(screen)
	}
	golden(t, "settings", screen)
	s.noWrite()
}

func TestSettingsHostChoiceAndOther(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press("3", keyEnter, keyRight, keyEnter)
	if got := <-s.client.writes; got != [2]string{"host", "mac"} {
		t.Fatalf("host write=%v", got)
	}
	s.waitFor("host  mac · config.toml")
	s.press(keyEnter, keyRight, "ctrl+a", "ctrl+k", "another", keyEnter)
	if got := <-s.client.writes; got != [2]string{"host", "another"} {
		t.Fatalf("other write=%v", got)
	}
	s.waitFor("host  another · config.toml")
	s.press(keyEnter, keyLeft, keyEscape)
	if screen := s.final(); !strings.Contains(screen, "host  another · config.toml") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsCoreRejectionKeepsEditorOpen(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.configData = "schema=1\n[repo]\nurl=\"local\"\n"
	s := start(t, client)
	s.waitFor("2 targets changed locally")
	s.settingRow(3)
	s.press(keyEnter, "Ada", keyEnter)
	if got := <-client.writes; got != [2]string{"git.identity.name", "Ada"} {
		t.Fatal(got)
	}
	s.waitFor("Not saved: The configuration or request is invalid", "Editing: Enter saves")
	s.press("X", keyEnter)
	if got := <-client.writes; got != [2]string{"git.identity.name", "AdaX"} {
		t.Fatalf("editor lost its draft: %v", got)
	}
	if screen := s.final(); !strings.Contains(screen, "Editing:") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsPickerMouseAndStrayEvents(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.settingRow(5)
	s.press(keyEnter)
	s.waitFor(" 30s [1m] 5m ")
	// At 80×24 the preset row is line 20. Clicking a preset changes only the draft.
	s.click(12, 20)
	s.waitFor("[5m]")
	s.mouse(tea.MouseButtonRight, tea.MouseActionPress, 2, 20)
	s.mouse(tea.MouseButtonLeft, tea.MouseActionRelease, 2, 20)
	s.mouse(tea.MouseButtonLeft, tea.MouseActionMotion, 2, 20)
	s.click(2, 19)
	s.press(keyEnter)
	if got := <-s.client.writes; got != [2]string{"sync.poll_interval", "5m"} {
		t.Fatalf("mouse write=%v", got)
	}
	s.waitFor("sync.poll_interval  5m · config.toml")
	s.press(keyEnter)
	s.click(2, 20)
	s.press(keyEscape)
	if screen := s.final(); !strings.Contains(screen, "sync.poll_interval  5m · config.toml") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsClosedPickerIgnoresText(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.settingRow(6)
	s.press(keyEnter, "false", keyEnter)
	screen := s.final()
	if strings.Contains(screen, "Editing:") || !strings.Contains(screen, "daemon.update_check  true · default") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsMigrationIsConditional(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.settings = append(client.settings, model.SettingValue{Key: "migration.legacy_roots", Value: "[]", LegacyDetected: true, Source: model.SourceDefault})
	s := start(t, client)
	s.waitFor("2 targets changed locally")
	s.settingRow(8)
	s.waitFor("Edit old paths with kitbag config set migration.legacy_roots")
	s.press(keyEnter)
	screen := s.final()
	if strings.Contains(screen, "Editing:") || strings.Contains(screen, "Enter: edit") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestSettingsStepperGolden(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.settingRow(5)
	s.press(keyEnter, keyRight)
	s.waitFor("[5m]")
	golden(t, "settings-stepper", s.final())
	s.noWrite()
}

func TestSettingsNarrowFooterKeepsAdvancedShortcut(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(tea.WindowSizeMsg{Width: MinWidth, Height: termHeight})
	s.press("3")
	s.waitFor("Advanced [+]", "a advanced")
	s.press(keyAdvanced)
	s.settingRow(10)
	s.press(keyEnter)
	s.waitFor("[500ms]", "Custom…")
	golden(t, "settings-narrow", s.final())
	s.noWrite()
}
