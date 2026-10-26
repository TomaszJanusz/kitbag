package tui

import "strings"

// Each terminal cell contains two square vertical subpixels. The front mask
// and three-step extrusion occupy the same five rows as the original wordmark.
const (
	logoDepth  = 3
	logoWidth  = 28
	logoHeight = 5
	cellPixels = 2
)

type logoShade uint8

const (
	logoEmpty logoShade = iota
	logoFront
	logoSide
	logoBottom
	logoShades
)

type logoCell struct{ upper, lower logoShade }

func wordmarkMask() []string {
	glyphs := [][]string{
		{"#  #", "# # ", "##  ", "# # ", "# # ", "#  #", "#  #"},
		{"#", " ", "#", "#", "#", "#", "#"},
		{" # ", " # ", "###", " # ", " # ", " # ", " ##"},
		{"#   ", "#   ", "### ", "#  #", "#  #", "#  #", "### "},
		{"    ", "    ", " ###", "#  #", "#  #", "#  #", " ###"},
		{"    ", "    ", " ###", "#  #", " ###", "   #", "### "},
	}
	mask := make([]string, len(glyphs[0]))
	for y := range mask {
		parts := make([]string, len(glyphs))
		for i, glyph := range glyphs {
			parts[i] = glyph[y]
		}
		mask[y] = strings.Join(parts, " ")
	}
	return mask
}

// maskToCells is pure and clips every layer to the requested cell rectangle.
// Offset units are square subpixels (one column, half a terminal row). Sweeping
// the mask to the offset joins the side faces; the front always wins overlaps.
func maskToCells(mask []string, dx, dy, width, height int) [][]logoCell {
	cols, rows := max(width, 0), max(height, 0)
	pixels := make(logoPixels, rows*cellPixels)
	for y := range pixels {
		pixels[y] = make([]logoShade, cols)
	}
	depth := max(abs(dx), abs(dy), 1)
	for step := 0; step <= depth; step++ {
		shade := logoSide
		if step == 0 {
			shade = logoFront
		}
		pixels.draw(mask, dx*step/depth, dy*step/depth, shade)
	}
	pixels.lowerEdges()

	cells := make([][]logoCell, rows)
	for y := range cells {
		cells[y] = make([]logoCell, cols)
		for x := range cells[y] {
			cells[y][x] = logoCell{pixels[y*cellPixels][x], pixels[y*cellPixels+1][x]}
		}
	}
	return cells
}

type logoPixels [][]logoShade

func (p logoPixels) draw(mask []string, dx, dy int, shade logoShade) {
	for y, line := range mask {
		for x := range len(line) {
			px, py := x+dx, y+dy
			if line[x] != '#' || py < 0 || py >= len(p) || px < 0 || px >= len(p[py]) {
				continue
			}
			if p[py][px] != logoFront {
				p[py][px] = shade
			}
		}
	}
}

// Only exposed lower shadow edges are darkened, never the accent face.
func (p logoPixels) lowerEdges() {
	for y, line := range p {
		for x, shade := range line {
			if shade == logoSide && (y+1 == len(p) || p[y+1][x] == logoEmpty) {
				p[y][x] = logoBottom
			}
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (s styles) logoLine(cells []logoCell) string {
	parts := make([]string, len(cells))
	for x, cell := range cells {
		glyph, fg, bg := "▀", cell.upper, cell.lower
		switch {
		case fg == logoEmpty && bg == logoEmpty:
			parts[x] = " "
			continue
		case fg == bg:
			glyph, bg = "█", logoEmpty
		case fg == logoEmpty:
			glyph, fg, bg = "▄", bg, logoEmpty
		}
		parts[x] = s.logo[fg][bg].Render(glyph)
	}
	return strings.Join(parts, "")
}
