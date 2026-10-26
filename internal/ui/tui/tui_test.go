package tui

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestStatusTabShowsTheSnapshot(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally", "Daemon, live updates", "unknown agent")
	s.client.events <- model.Event{Kind: model.EventUpdateAvailable}
	s.waitFor("A newer kitbag release is available")
	golden(t, "status", s.final())
}

func TestStatusShowsToolTooOldAndDisablesSync(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.status = model.StatusSnapshot{Repository: "/repo", ToolTooOld: &model.ToolTooOld{Required: "0.3.0", Current: "0.1.0"}}
	client.statusErr = &model.Error{Kind: model.ErrToolTooOld, Detail: "raw core detail: manifest requires 0.3.0", ExitCode: 4}
	s := start(t, client)
	s.waitFor("requires kitbag 0.3.0 or newer; running 0.1.0", "[s] Sync now (unavailable)")
	s.press(keySync)
	screen := s.final()
	if strings.Contains(screen, "raw core detail") || !strings.Contains(screen, "Showing the last known status") {
		t.Fatalf("problem text must come from the catalog:\n%s", screen)
	}
	if strings.Contains(screen, "Syncing") {
		t.Fatalf("sync started although a newer tool is required:\n%s", screen)
	}
}

func TestProblemsUseCatalogText(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.statusErr = &model.Error{Kind: model.ErrUnavailable, Detail: "dial unix /run/kitbag.sock: connection refused"}
	s := start(t, client)
	s.waitFor("kitbag is not reachable")
	if screen := s.final(); strings.Contains(screen, "connection refused") {
		t.Fatalf("raw diagnostic shown:\n%s", screen)
	}
}

func TestSyncNowShowsLiveProgress(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	loads := s.client.statusCalls.Load()
	s.press(keySync)
	s.client.phases <- "pull"
	s.waitFor("Syncing: pulling the repository")
	s.client.phases <- "apply"
	s.waitFor("Syncing: applying changes")
	close(s.client.phases)
	s.waitFor("Last sync from here: succeeded, 3 changed")
	if s.client.statusCalls.Load() <= loads {
		t.Fatal("status was not reloaded after the sync")
	}
	s.press("4")
	s.waitFor("info Sync: pulling the repository", "info Sync succeeded, 3 changed")
}

// The in-process services share the state lock: loads, syncs and writes must
// never overlap, and a burst of reloads collapses into the newest request.
func TestCommandsRunOneAtATime(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.delay = 10 * time.Millisecond
	s := start(t, client)
	s.waitFor("2 targets changed locally")
	for range 5 {
		s.press(keyRefresh)
	}
	s.press(keySync)
	close(client.phases)
	s.waitFor("Last sync from here: succeeded, 3 changed")
	s.press("3")
	s.waitFor("host  laptop · config.toml")
	if client.overlapped.Load() {
		t.Fatal("client calls overlapped")
	}
	if calls := client.statusCalls.Load(); calls > 4 {
		t.Fatalf("%d status loads; queued reloads were not superseded", calls)
	}
}

func TestDriftDetailOpensAndCloses(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("2")
	s.waitFor("2 targets differ", "> /home/user/.claude/CLAUDE.md", "Update needed")
	s.press(keyDown, keyEnter)
	s.waitFor("Planned actions", "Back up the current file", "Content diffs are not available")
	s.press(keyEscape)
	s.waitFor("Enter: details")
	s.press(keyEnter)
	s.waitFor("Backup: /home/user/.local/state/kitbag/backup/AGENTS.md", "Esc: back to the list")
	golden(t, "drift-detail", s.final())
}

func TestDriftEmptyState(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.drift = model.DriftSnapshot{Daemon: true}
	s := start(t, client)
	s.press(keyNext)
	s.waitFor("No drift: every target is in sync")
	s.press(keyEnter)
	if screen := s.final(); strings.Contains(screen, "Planned actions") {
		t.Fatalf("detail opened without a target:\n%s", screen)
	}
}

func TestSettingsEditValidateAndSave(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press("3")
	s.waitFor("Precedence:", "host  laptop · config.toml")
	// Polling uses a duration stepper; 60s matches the 1m preset.
	s.press(keyDown, keyDown, keyDown, keyDown, keyDown, keyEnter, keyRight, keyEnter)
	if got := <-s.client.writes; got != [2]string{"sync.poll_interval", "5m"} {
		t.Fatalf("write=%v", got)
	}
	s.waitFor("sync.poll_interval  5m · config.toml")
	s.press(keyEnter, keyRight, keyRight, keyRight, "ctrl+a", "ctrl+k", "soon", keyEnter)
	s.waitFor("Allowed: duration from 1s to 24h", "Editing: Enter saves")
	golden(t, "settings-invalid", s.final())
	select {
	case got := <-s.client.writes:
		t.Fatalf("invalid draft written: %v", got)
	default:
	}
}

func TestSettingsShowsFailedSaveAndOverride(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.settingErr = &model.Error{Kind: model.ErrBusy, Detail: "state lock held"}
	s := start(t, client)
	s.press("3")
	s.waitFor("host  laptop · config.toml")
	s.press(keyEnter, keyLeft, "-2", keyEnter)
	<-client.writes
	s.waitFor("Not saved: Another kitbag operation is running")
	s.press(keyEscape)
	for range 7 {
		s.press(keyDown)
	}
	s.waitFor("environment KITBAG_MODE overrides config.toml")
}

func TestSettingsUnsetRevertAndMaskSecrets(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.press(keyPrevious, keyPrevious)
	s.waitFor("host  laptop · config.toml")
	s.press(keyUnset)
	if got := <-s.client.writes; got != [2]string{"host", ""} {
		t.Fatalf("unset=%v", got)
	}
	s.press(keyVimDown, keyEnter, "https://x")
	s.waitFor("> *********", "repo.url *")
	s.press(keyEscape)
	s.waitFor("repo.url  unset · default")
	s.press(keyEnter, "h", keyEnter)
	if got := <-s.client.writes; got != [2]string{"repo.url", "h"} {
		t.Fatalf("write=%v", got)
	}
	s.waitFor("repo.url  [redacted] · config.toml")
	if screen := s.final(); strings.Contains(screen, "https://x") {
		t.Fatalf("secret shown:\n%s", screen)
	}
}

func TestLogFollowsWithABoundedBuffer(t *testing.T) {
	t.Parallel()
	s := start(t, newFake(), func(o *Options) { o.LogCapacity = 8 })
	// Ten lines leave six activity rows.
	s.tm.Send(tea.WindowSizeMsg{Width: termWidth, Height: MinHeight})
	s.press("4")
	s.waitFor("Connected to the daemon")
	for range 8 {
		s.client.events <- model.Event{Kind: model.EventDriftDetected}
	}
	s.client.events <- model.Event{Kind: model.EventSyncFinished, ExitCode: 1, Changed: 7}
	s.waitFor("2 older entries dropped", "warn A sync finished with warnings, 7 changed")
	s.press(keyUp)
	s.waitFor("Scrolling; press f to follow new entries")
	// A paused view keeps its entries although the full buffer drops one.
	s.client.events <- model.Event{Kind: model.EventUpdateAvailable}
	s.waitFor("3 older entries dropped")
	s.press(keyFollow)
	s.waitFor("Following new entries", "info A newer kitbag release is available")
	screen := s.final()
	golden(t, "log", screen)
	if got := strings.Count(screen, "warn The daemon detected drift"); got != 4 {
		t.Fatalf("showing %d drift entries:\n%s", got, screen)
	}
}

func TestHelpOverlayToggles(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press(keyHelp)
	s.waitFor("Keyboard", "q or Ctrl-C: quit", "a on Settings: expand or collapse Advanced")
	golden(t, "help", s.final())
}

func TestResizeShowsMinimumAndRecovers(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(tea.WindowSizeMsg{Width: 30, Height: 8})
	s.waitFor("Enlarge the terminal to at", "(now 30×8)")
	s.tm.Send(tea.WindowSizeMsg{Width: MinWidth, Height: MinHeight + 2})
	s.waitFor("Repository")
	screen := s.final()
	golden(t, "status-narrow", screen)
	for _, line := range strings.Split(screen, "\n") {
		if width := len([]rune(plainText([]byte(line)))); width > MinWidth {
			t.Fatalf("line wider than the terminal (%d): %q", width, line)
		}
	}
}

// colourSGR matches SGR sequences that set a foreground or background colour.
var colourSGR = regexp.MustCompile(`\x1b\[(?:[0-9;]*;)?(?:3[0-9]|4[0-9]|9[0-7]|10[0-7])(?:;[0-9;]*)?m`)

func TestNoColourEmitsNoColourSequences(t *testing.T) {
	t.Parallel()
	for _, noColour := range []bool{false, true} {
		s := start(t, newFake(), func(o *Options) { o.Profile, o.NoColour = termenv.TrueColor, noColour })
		s.waitFor("2 targets changed locally")
		s.press("4")
		s.waitFor("Connected to the daemon")
		screen := s.final()
		if coloured := colourSGR.MatchString(screen); coloured == noColour {
			t.Fatalf("NoColour=%v coloured=%v:\n%q", noColour, coloured, screen)
		}
	}
}

func TestQuitClosesTheSubscription(t *testing.T) {
	t.Parallel()
	for _, key := range []string{keyQuit, keyCtrlC} {
		client := newFake()
		m := New(Options{Runner: model.Runner{Client: client, Now: clock}})
		s := &session{t: t, m: m, client: client}
		s.tm = startModel(t, m)
		for client.subscribed.Load() == 0 {
			time.Sleep(time.Millisecond)
		}
		s.press(key)
		s.tm.WaitFinished(t)
		closed := make(chan struct{})
		go func() { m.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(waitLimit):
			t.Fatal("workers still running after quit")
		}
		if client.subscribed.Load() != 1 || client.unsubscribed.Load() != 1 {
			t.Fatalf("%s: subscribed=%d closed=%d", key, client.subscribed.Load(), client.unsubscribed.Load())
		}
	}
}

func TestRunRestoresTheTerminalAfterAPanic(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.panicStatus = true
	reader := idleInput(t)
	var output strings.Builder
	err := Run(t.Context(), Options{Runner: model.Runner{Client: client}}, reader, &output)
	if !errors.Is(err, tea.ErrProgramPanic) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(output.String(), "\x1b[?1049l") {
		t.Fatalf("alternate screen not left: %q", output.String())
	}
}

func TestRunEndsWithItsContext(t *testing.T) {
	t.Parallel()
	client := newFake()
	reader := idleInput(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Options{Runner: model.Runner{Client: client}}, reader, io.Discard) }()
	for client.subscribed.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(waitLimit):
		t.Fatal("Run did not return after its context ended")
	}
	if client.unsubscribed.Load() != 1 {
		t.Fatal("subscription still open")
	}
}
