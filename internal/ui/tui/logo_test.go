package tui

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

func TestMaskToCellsOffsetsAndShades(t *testing.T) {
	t.Parallel()
	got := maskToCells([]string{"##", "##"}, 2, 2, 4, 2)
	want := [][]logoCell{
		{{logoFront, logoFront}, {logoFront, logoFront}, {logoEmpty, logoSide}, {}},
		{{}, {logoBottom, logoEmpty}, {logoSide, logoBottom}, {logoSide, logoBottom}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("offset/shades: got %v, want %v", got, want)
	}
	zero := maskToCells([]string{"#"}, 0, 0, 1, 1)
	if zero[0][0] != (logoCell{logoFront, logoEmpty}) {
		t.Fatal("zero offset changed the front", zero)
	}
}

func TestMaskToCellsClipsEveryLayer(t *testing.T) {
	t.Parallel()
	for _, offset := range [][2]int{{3, 3}, {-3, -3}, {-2, 3}, {3, -2}, {0, 0}} {
		for width := range 6 {
			for height := range 5 {
				cells := maskToCells([]string{"# #", "", "###"}, offset[0], offset[1], width, height)
				if len(cells) != height {
					t.Fatal("shadow drawn outside vertical bounds", cells)
				}
				for _, row := range cells {
					if len(row) != width {
						t.Fatal("shadow drawn outside horizontal bounds", row)
					}
				}
			}
		}
	}
	// Clipping discards pixels instead of piling them onto the right/bottom edge.
	cells := maskToCells([]string{"#"}, 3, 3, 2, 1)
	if cells[0][1] != (logoCell{logoEmpty, logoBottom}) {
		t.Fatal("clipped shadow accumulated at edge", cells)
	}
}

func TestMaskToCellsFrontWins(t *testing.T) {
	t.Parallel()
	mask := wordmarkMask()
	cells := maskToCells(mask, logoDepth, logoDepth, logoWidth, logoHeight)
	for y, line := range mask {
		for x := range len(line) {
			if line[x] != '#' {
				continue
			}
			shade := cells[y/cellPixels][x].upper
			if y%cellPixels != 0 {
				shade = cells[y/cellPixels][x].lower
			}
			if shade != logoFront {
				t.Fatalf("shadow overwrote front face at %d,%d: %d", x, y, shade)
			}
		}
	}
	golden(t, "v2-logo-shades", shadeIndices(cells))
}

func shadeIndices(cells [][]logoCell) string {
	result := make([]string, 0, cellPixels*len(cells))
	shades := []rune{' ', '█', '▓', '▒'}
	for _, row := range cells {
		for _, lower := range []bool{false, true} {
			var line []rune
			for _, cell := range row {
				shade := cell.upper
				if lower {
					shade = cell.lower
				}
				line = append(line, shades[shade])
			}
			result = append(result, string(line))
		}
	}
	return strings.Join(result, "\n")
}

func TestLogoShadeProfilesAndHalfBlocks(t *testing.T) {
	t.Parallel()
	cells := []logoCell{{logoFront, logoSide}, {logoEmpty, logoBottom}, {logoFront, logoFront}}
	for _, tc := range []struct {
		codes   []string
		profile Profile
	}{
		{[]string{"38;2;233;184;113", "48;2;142;99;56", "38;2;80;56;31"}, termenv.TrueColor},
		{[]string{"38;5;179", "48;5;95", "38;5;58"}, termenv.ANSI256},
		{[]string{"33", "100", "90"}, termenv.ANSI},
	} {
		line := newStyles(tc.profile).logoLine(cells)
		if plainText([]byte(line)) != "▀▄█" {
			t.Fatal("lost half-block geometry", line)
		}
		for _, code := range tc.codes {
			if !strings.Contains(line, code) {
				t.Fatalf("profile %v lacks shade %s: %q", tc.profile, code, line)
			}
		}
	}
	if line := newStyles(termenv.Ascii).logoLine(cells); ansiPattern.MatchString(line) {
		t.Fatal("NO_COLOR emitted colour", line)
	}
}

func TestLogoFootprintAndResize(t *testing.T) {
	t.Parallel()
	m := homeProjection(t, 120, 35)
	for _, width := range []int{1, logoWidth - 1, logoWidth, logoWidth + 1, 60, 79, 80, 120} {
		m.width = width
		for _, profile := range []Profile{termenv.Ascii, termenv.TrueColor, termenv.ANSI} {
			m.styles = newStyles(profile)
			lines := m.logo()
			wantRows := 1
			if profile != termenv.Ascii && width >= pixelMinWidth {
				wantRows = logoHeight
			}
			if len(lines) != wantRows {
				t.Fatal("extrusion changed the logo footprint", lines)
			}
			assertScreenFits(t, strings.Join(lines, "\n"), width, wantRows)
		}
	}
}

func rawGolden(t *testing.T, name, screen string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	got := []byte(screen + "\n")
	if flag.Lookup(updateFlag).Value.String() == "true" {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("raw ANSI screen %s differs; review with -%s", name, updateFlag)
	}
}
