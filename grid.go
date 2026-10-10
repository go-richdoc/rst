// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"

	docrst "github.com/go-docutils/docutils/rst"
)

// A grid table is a RECTANGLE of slots, not a list of rows: once a cell spans
// rows, the cells of the row below no longer start at column 0, and the
// horizontal rule between the two rows is missing exactly where the spanning
// cell continues. Writing one is therefore a placement problem followed by a
// drawing problem, which is what this file is.
//
// The shape it draws is docutils' own, confirmed against its GridTableParser
// on the model it returns -- `(morerows, morecols, offset, lines)` per cell
// and `None` in every slot a span covers:
//
//	+-------+-------------------+
//	|string | Python 3.2        |
//	|size   +--------+----------+
//	|       | 16-bit | 32-bit   |
//	+-------+--------+----------+
//
// Two things in that picture are easy to get wrong and are what the drawing
// rule below is built around. The spanning cell's own text keeps flowing
// across the rule that interrupts it ("size" sits ON the border line), and the
// "+" opening that rule belongs to the cells to its RIGHT, not to the row.

// gridCell is one cell PLACED: the rectangle it occupies in grid coordinates,
// with its text already rendered.
type gridCell struct {
	text string
	row  int
	col  int
	rows int
	span int
}

// placeGrid assigns every cell a rectangle, skipping the slots a span from an
// earlier row has already claimed, and fills whatever is left over with empty
// cells -- a row shorter than the table, or a slot no cell reached, is a HOLE,
// and a hole has no reST spelling at all.
//
// It returns the cells, the owner index of every slot -- which is what the
// drawing pass asks all its questions of -- and the grid's own two dimensions,
// neither of which is the caller's row count once [withoutPhantomRows] has
// had its say.
func placeGrid(rows [][]spanCell) (cells []gridCell, owner [][]int, nrows, ncols int) {
	rows = withoutPhantomRows(rows)
	nrows = len(rows)
	taken := make([]map[int]bool, nrows)
	for r := range taken {
		taken[r] = map[int]bool{}
	}
	claim := func(c gridCell) {
		for dr := 0; dr < c.rows; dr++ {
			for dc := 0; dc < c.span; dc++ {
				taken[c.row+dr][c.col+dc] = true
			}
		}
	}
	for r, row := range rows {
		col := 0
		for _, sc := range row {
			for taken[r][col] {
				col++
			}
			// A RowSpan running past the last row is clamped rather than
			// refused: the tree is what some other parser produced, and a
			// rectangle that fits is always drawable.
			span := sc.rows
			if r+span > nrows {
				span = nrows - r
			}
			if span < 1 {
				span = 1
			}
			c := gridCell{text: sc.text, row: r, col: col, rows: span, span: sc.span}
			cells = append(cells, c)
			claim(c)
			col += sc.span
			if col > ncols {
				ncols = col
			}
		}
	}
	for r := 0; r < nrows; r++ {
		for col := 0; col < ncols; col++ {
			if !taken[r][col] {
				c := gridCell{row: r, col: col, rows: 1, span: 1}
				cells = append(cells, c)
				claim(c)
			}
		}
	}
	owner = make([][]int, nrows)
	for r := range owner {
		owner[r] = make([]int, ncols)
	}
	for i, c := range cells {
		for dr := 0; dr < c.rows; dr++ {
			for dc := 0; dc < c.span; dc++ {
				owner[c.row+dr][c.col+dc] = i
			}
		}
	}
	return cells, owner, nrows, ncols
}

// withoutPhantomRows drops every row that holds no cells OF ITS OWN, shrinking
// the spans that covered it.
//
// Such a row cannot be written: a row span whose second row is empty draws
// exactly like one tall cell, so the reconstruction reads one row where the
// tree held two, and the written table is not even a fixed point (the tall
// cell's blank lines come back as a one-line row). Nothing is lost by dropping
// it, because nothing of it was ever drawn.
//
// It is also not a shape an author writes. The one file in the corpus that
// produces one -- PEP 669 -- has a typo in its table, "+ Two or more |" where
// a "|" belongs, and docutils answers by reading the final rule's row as
// covered by a span from above. The tree is then describing the typo, not the
// table.
func withoutPhantomRows(rows [][]spanCell) [][]spanCell {
	keep := make([]int, 0, len(rows))
	for r, row := range rows {
		if len(row) > 0 {
			keep = append(keep, r)
		}
	}
	if len(keep) == len(rows) {
		return rows
	}
	out := make([][]spanCell, 0, len(keep))
	for i, r := range keep {
		row := make([]spanCell, len(rows[r]))
		copy(row, rows[r])
		for j := range row {
			// The span is in OLD row numbers: count how many of the rows it
			// covered are still there.
			n := 0
			for _, k := range keep[i:] {
				if k >= r+row[j].rows {
					break
				}
				n++
			}
			row[j].rows = n
		}
		out = append(out, row)
	}
	return out
}

// gridWidths establishes column widths from the cells that span ONE column,
// then gives a wider spanning cell's overflow to the last column it covers.
// Simple over an even split: what matters is that the merged content fits,
// not that the columns under it are balanced.
func gridWidths(cells []gridCell, ncols int) []int {
	widths := make([]int, ncols)
	for _, c := range cells {
		if c.span == 1 {
			if w := cellWidth(c.text); w > widths[c.col] {
				widths[c.col] = w
			}
		}
	}
	for i := range widths {
		if widths[i] < 1 {
			widths[i] = 1
		}
	}
	for _, c := range cells {
		if c.span > 1 {
			if need := cellWidth(c.text) - spanTextWidth(widths, c.col, c.span); need > 0 {
				widths[c.col+c.span-1] += need
			}
		}
	}
	return widths
}

// gridHeights is the same shape of computation one axis over, with one
// difference that only the vertical axis has: a cell spanning rows also owns
// the BORDER lines between them, so its room is the rows' heights plus the
// rules it covers.
func gridHeights(cells []gridCell, lines [][]string, nrows int) []int {
	heights := make([]int, nrows)
	for r := range heights {
		heights[r] = 1
	}
	for i, c := range cells {
		if c.rows == 1 && len(lines[i]) > heights[c.row] {
			heights[c.row] = len(lines[i])
		}
	}
	for i, c := range cells {
		if c.rows > 1 {
			room := c.rows - 1
			for r := c.row; r < c.row+c.rows; r++ {
				room += heights[r]
			}
			if n := len(lines[i]); n > room {
				heights[c.row+c.rows-1] += n - room
			}
		}
	}
	return heights
}

// gridBoundary is the character where two slots meet on one output line, given
// what owns each side -- an owning cell index, [edgeSlot] for a slot the line
// cuts a horizontal rule through, or [outsideGrid] past the table itself.
//
// Every character of a grid table's frame comes from this one question, which
// is why a row span needs no special case anywhere else: the rule between two
// rows simply stops being drawn where one cell owns both sides of it.
const (
	edgeSlot    = -1
	outsideGrid = -2
)

func gridBoundary(left, right int) byte {
	switch {
	case left == right && left >= 0:
		// Inside ONE cell: a column span's interior on a content line, or a
		// row span's interior where a rule would otherwise run.
		return ' '
	case left >= 0 && right >= 0, left >= 0 && right == outsideGrid, left == outsideGrid && right >= 0:
		return '|'
	default:
		return '+'
	}
}

// renderGrid draws the table. headerRule is the row boundary the "=" rule
// belongs on (1 when the table has a header row), or 0 for none.
func renderGrid(cells []gridCell, owner [][]int, widths, heights []int, lines [][]string, headerRule int) string {
	nrows, ncols := len(heights), len(widths)
	yoff := make([]int, nrows+1)
	for r := 0; r < nrows; r++ {
		yoff[r+1] = yoff[r] + heights[r] + 1
	}
	// A line's whole structure is "which cell, if any, is this slot INSIDE at
	// this line" -- inside meaning strictly between the cell's own two rules,
	// so a cell's first and last lines are the rules themselves and everything
	// between them, border lines included, belongs to the cell.
	inside := make([]int, ncols)
	var b strings.Builder
	for y := 0; y <= yoff[nrows]; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		rule := byte('-')
		if headerRule > 0 && y == yoff[headerRule] {
			rule = '='
		}
		for c := range inside {
			inside[c] = edgeSlot
		}
		for r := 0; r < nrows; r++ {
			if yoff[r] < y && y < yoff[r+1] {
				copy(inside, owner[r])
				break
			}
			// A row span is interior to this line too when the line is one of
			// the rules it crosses, which is the case yoff[r] < y < yoff[r+1]
			// cannot see.
			if y == yoff[r] {
				for c := 0; c < ncols; c++ {
					if r > 0 && owner[r][c] == owner[r-1][c] {
						inside[c] = owner[r][c]
					}
				}
				break
			}
		}
		col := 0
		for col < ncols {
			left := outsideGrid
			if col > 0 {
				left = inside[col-1]
			}
			b.WriteByte(gridBoundary(left, inside[col]))
			if inside[col] == edgeSlot {
				b.WriteString(strings.Repeat(string(rule), widths[col]+2))
				col++
				continue
			}
			i := inside[col]
			cell := cells[i]
			text := ""
			if n := y - yoff[cell.row] - 1; n >= 0 && n < len(lines[i]) {
				text = lines[i][n]
			}
			// pad is never negative: gridWidths grew these very columns to fit
			// this cell's widest line.
			pad := spanTextWidth(widths, cell.col, cell.span) - docrst.TableColumnWidth(text)
			b.WriteString(" " + text + strings.Repeat(" ", pad) + " ")
			col = cell.col + cell.span
		}
		b.WriteByte(gridBoundary(inside[ncols-1], outsideGrid))
	}
	return b.String()
}
