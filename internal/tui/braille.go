package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Braille cells pack a 2x4 grid of dots into one rune starting at U+2800. Each
// dot maps to a bit; lighting dots ORs their bits onto the base rune. This
// gives 2x horizontal and 4x vertical sub-character resolution, which is what
// makes the radar's circles and polygon edges look smooth.
const (
	brailleBase  = 0x2800
	brailleCellW = 2 // sub-dots per cell, horizontally
	brailleCellH = 4 // sub-dots per cell, vertically
)

// brailleDotBits maps a (col,row) sub-dot within a cell to its braille bit.
// Layout per the Unicode braille pattern:
//
//	(0,0)=0x01 (1,0)=0x08
//	(0,1)=0x02 (1,1)=0x10
//	(0,2)=0x04 (1,2)=0x20
//	(0,3)=0x40 (1,3)=0x80
var brailleDotBits = [brailleCellW][brailleCellH]rune{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// brailleCanvas is a fixed-size pixel surface rendered as braille runes. Each
// pixel may carry a color; the cell takes the color of its most recently set
// pixel (good enough for distinct radar elements that rarely share a cell).
type brailleCanvas struct {
	cols, rows int // size in character cells
	pxW, pxH   int // size in sub-dot pixels
	bits       []rune
	colors     []color.Color
}

// newBrailleCanvas allocates a canvas wide×tall character cells.
func newBrailleCanvas(cols, rows int) *brailleCanvas {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return &brailleCanvas{
		cols:   cols,
		rows:   rows,
		pxW:    cols * brailleCellW,
		pxH:    rows * brailleCellH,
		bits:   make([]rune, cols*rows),
		colors: make([]color.Color, cols*rows),
	}
}

// set lights the pixel at (px,py) with the given color. Out-of-bounds pixels
// are ignored so callers need not clip.
func (c *brailleCanvas) set(px, py int, col color.Color) {
	if px < 0 || py < 0 || px >= c.pxW || py >= c.pxH {
		return
	}
	cellX, cellY := px/brailleCellW, py/brailleCellH
	dotX, dotY := px%brailleCellW, py%brailleCellH
	idx := cellY*c.cols + cellX
	c.bits[idx] |= brailleDotBits[dotX][dotY]
	c.colors[idx] = col
}

// line draws a straight line between two pixel points (Bresenham).
func (c *brailleCanvas) line(x0, y0, x1, y1 int, col color.Color) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := step(x0, x1)
	sy := step(y0, y1)
	err := dx + dy
	for {
		c.set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// render returns the canvas as newline-joined, colored braille rows. Cells with
// no lit dots render as a blank space to keep the surface visually clean.
func (c *brailleCanvas) render() string {
	var b strings.Builder
	for y := 0; y < c.rows; y++ {
		for x := 0; x < c.cols; x++ {
			idx := y*c.cols + x
			if c.bits[idx] == 0 {
				b.WriteByte(' ')
				continue
			}
			r := rune(brailleBase) + c.bits[idx]
			if col := c.colors[idx]; col != nil {
				b.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(r)))
			} else {
				b.WriteRune(r)
			}
		}
		if y < c.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func step(from, to int) int {
	if from < to {
		return 1
	}
	return -1
}
