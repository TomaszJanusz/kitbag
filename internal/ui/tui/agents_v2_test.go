package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

func TestAgentsHomeButtonFlow(t *testing.T) {
	t.Parallel()
	projection := drive(t, withAgents(newFake()), termWidth, termHeight)
	projection.press("5")
	hit := projection.m.actionLayout().hits[0]
	s := startHome(t, withAgents(newFake()))
	s.waitFor("2 targets changed locally")
	s.press("a")
	s.waitFor("kitbag › Agents", "claude/main")
	s.click(hit.rect.X+len(indent), hit.rect.Y)
	s.waitFor("Step 1/7", "Add an agent instance")
	s.press(keyEscape)
	s.waitFor("kitbag › Agents")
	s.press(keyEscape)
	s.waitFor("? help · q quit")
	if screen := s.final(); !s.m.home || s.m.windows.Len() != 0 {
		t.Fatal(screen)
	}
}

func TestAgentsV2ScreensAndChoiceDialog(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 20}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			d := drive(t, withAgents(newFake()), size[0], size[1])
			d.press("5")
			d.expect("kitbag › Agents", "claude/main", "Add agent")
			d.golden(fmt.Sprintf("v2-agents-%dx%d", size[0], size[1]))
			d.press(keyAdd)
			d.expect("Step 1/7", "esc", "> [ Next ]")
			if size[1] > MinHeight {
				d.expect("(*) claude")
			}
			d.golden(fmt.Sprintf("v2-agents-choices-%dx%d", size[0], size[1]))
		})
	}
}

func TestAgentsPrimaryButtonUsesSharedStyle(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5")
	d.m.styles = newStyles(termenv.TrueColor)
	buttons := strings.Join(d.m.actionLayout().lines, "\n")
	if !strings.Contains(buttons, "48;2;233;184;") || !strings.Contains(buttons, "38;2;24;24;24") {
		t.Fatal("Add is missing the warm fill/dark text", buttons)
	}
	plain := plainText([]byte(buttons))
	if !strings.Contains(plain, "[ Edit ]") || !strings.Contains(plain, "[ Remove ]") {
		t.Fatal("secondary actions are not buttons", plain)
	}
}
