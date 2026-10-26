package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestDialogPresetScreens(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 20}, {40, 10}} {
		for _, preset := range []string{"confirm", "form", "finish", "info"} {
			t.Run(fmt.Sprintf("%s-%dx%d", preset, size[0], size[1]), func(t *testing.T) {
				t.Parallel()
				m := loadedSettingsModel(t, size[0], size[1])
				m.ShowDialog(dialogPreset(preset))
				screen := strings.Join(m.overlayWindows(make([]string, size[1])), "\n")
				assertScreenFits(t, screen, size[0], size[1])
				if !strings.Contains(screen, "esc") {
					t.Fatal("escape hint is hidden")
				}
				golden(t, "v2-dialog-"+t.Name()[len("TestDialogPresetScreens/"):], screen)
			})
		}
	}
}

func dialogPreset(preset string) model.DialogView {
	switch preset {
	case "form":
		return formFixture()
	case "finish":
		view := formFixture()
		view.Step = view.Steps
		view.Buttons[1].Label = text.New(text.DialogFinish)
		return view
	case "info":
		return model.DialogView{
			ID: "info", Title: text.New(text.HelpTitle), Message: text.Plain(strings.Repeat("Keyboard help body\n", 40)), CloseOnAction: true,
			Buttons: []model.Control{{Label: text.New(text.DialogOK), Enabled: true, Intent: model.DismissDialog{ID: "info"}}},
		}
	default:
		return confirmFixture("Remove stored setting?")
	}
}

func TestColourPanelHasNoHeavyFrame(t *testing.T) {
	t.Parallel()
	m := New(Options{Profile: termenv.TrueColor})
	defer m.Close()
	m.width, m.height = 80, 24
	m.ShowDialog(confirmFixture("Confirmation"))
	panel := strings.Join(m.windowLayout(m.windows.top()).lines, "\n")
	if strings.Contains(panel, "+---") || !strings.Contains(panel, "48;2;36;36;36") || !strings.Contains(panel, "48;2;233;184;") {
		t.Fatal("panel/primary colour or border differs:", panel)
	}
}
