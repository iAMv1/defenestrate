package tui

import (
	"sort"
	"strings"

	"github.com/iAMv1/defenestrate/internal/analyze"
)

// TreeMapItem is one leaf of the map.
type TreeMapItem struct {
	Label string
	Path  string
	Size  int64
}

// TreeMapTile is a laid-out rectangle in character-grid coordinates.
type TreeMapTile struct {
	Item            TreeMapItem
	Col, Row, W, H  int
	Rank            int // 0 = biggest
}

type tmItem struct {
	label string
	path  string
	area  float64 // normalized: sum == total grid area
}

// BuildTreeMap lays items into a cols×rows grid using the squarified-treemap
// algorithm (Bruls et al.): rows are grown along the shorter side while the
// worst aspect ratio improves, then committed. Items must fit a readable
// minimum footprint or they are dropped — an unreadable sliver is noise.
func BuildTreeMap(items []analyze.Entry, cols, rows int) []TreeMapTile {
	const minW, minH = 8, 2
	if cols < minW*2 || rows < minH*3 || len(items) == 0 {
		return nil
	}
	var sized []tmItem
	var total float64
	for _, e := range items {
		if e.Size > 0 {
			sized = append(sized, tmItem{label: e.Name, path: e.Path, area: float64(e.Size)})
			total += float64(e.Size)
		}
	}
	if len(sized) == 0 || total <= 0 {
		return nil
	}
	gridArea := float64(cols * rows)
	for i := range sized {
		sized[i].area = sized[i].area / total * gridArea
	}
	sort.SliceStable(sized, func(i, j int) bool { return sized[i].area > sized[j].area })

	type rect struct{ x, y, w, h int }
	cur := rect{0, 0, cols, rows}

	var tiles []TreeMapTile
	rank := 0
	for len(sized) > 0 && cur.w >= minW && cur.h >= minH {
		vertical := cur.w <= cur.h // strip runs along the shorter side
		sideLen := cur.w           // horizontal strip: thickness counted in rows
		if vertical {
			sideLen = cur.h
		}

		row := []tmItem{sized[0]}
		rowArea := sized[0].area
		best := worstRatio(row, rowArea, sideLen)
		n := 1
		for n < len(sized) {
			candRow := append(append([]tmItem{}, row...), sized[n])
			w := worstRatio(candRow, rowArea+sized[n].area, sideLen)
			if w > best {
				break
			}
			row, rowArea, best = candRow, rowArea+sized[n].area, w
			n++
		}

		thickness := int(rowArea / float64(sideLen))
		if vertical {
			if thickness < minW {
				thickness = minW
			}
			if thickness > cur.w {
				thickness = cur.w
			}
		} else {
			if thickness < minH {
				thickness = minH
			}
			if thickness > cur.h {
				thickness = cur.h
			}
		}

		// Lay the row out along the strip.
		cx, cy := cur.x, cur.y
		remW, remH := cur.w, cur.h // clamp targets: never leave the rect
		for i, it := range row {
			frac := it.area / rowArea
			t := TreeMapTile{Item: TreeMapItem{it.label, it.path, int64(it.area * total / gridArea)}, Rank: rank}
			rank++
			last := i == len(row)-1
			if vertical {
				hh := int(frac * float64(remH))
				if last || hh > remH {
					hh = remH
				}
				if hh < minH && len(row) > 1 && remH >= minH {
					hh = minH
				}
				t.Col, t.Row, t.W, t.H = cx, cy, thickness, hh
				cy += hh
				remH -= hh
			} else {
				ww := int(frac * float64(remW))
				if last || ww > remW {
					ww = remW
				}
				if ww < minW && len(row) > 1 && remW >= minW {
					ww = minW
				}
				t.Col, t.Row, t.W, t.H = cx, cy, ww, thickness
				cx += ww
				remW -= ww
			}
			if t.W <= 0 || t.H <= 0 {
				continue
			}
			if t.W >= minW && t.H >= minH {
				tiles = append(tiles, t)
			}
		}

		if vertical {
			cur.x += thickness
			cur.w -= thickness
		} else {
			cur.y += thickness
			cur.h -= thickness
		}
		sized = sized[n:]
	}
	return tiles
}

// worstRatio is the classic worst-aspect-ratio metric for a candidate row.
func worstRatio(row []tmItem, rowArea float64, sideLen int) float64 {
	if sideLen <= 0 || rowArea <= 0 {
		return 1e18
	}
	thick := rowArea / float64(sideLen)
	if thick <= 0 {
		return 1e18
	}
	worst := 1e18
	for _, it := range row {
		length := it.area / thick
		a, b := length, thick
		if a < b {
			a, b = b, a
		}
		if r := a / b; r < worst {
			worst = r
		}
	}
	return worst
}

// RenderTreeMap paints tiles into a char grid; label goes on the tile's first
// usable line, truncated to fit. The tile whose Path matches selected gets a
// corner marker.
func RenderTreeMap(tiles []TreeMapTile, cols, rows int, selected string) string {
	grid := make([][]rune, rows)
	for r := range grid {
		grid[r] = []rune(strings.Repeat(" ", cols))
	}
	for _, t := range tiles {
		for y := t.Row; y < t.Row+t.H && y < rows; y++ {
			fill := "▓"
			if t.Rank%3 == 1 {
				fill = "▒"
			} else if t.Rank%3 == 2 {
				fill = "░"
			}
			for x := t.Col; x < t.Col+t.W && x < cols; x++ {
				grid[y][x] = []rune(fill)[0]
			}
		}
	}
	for _, t := range tiles {
		label := t.Item.Label
		maxL := t.W - 1
		if maxL <= 0 || t.H < 1 || t.Col+1 >= cols || t.Row+1 >= rows {
			continue
		}
		if len(label) > maxL {
			label = label[:maxL-1] + "…"
		}
		line := []rune(label)
		y := t.Row + 1
		if t.H > 2 { // center-ish when tall enough
			y = t.Row + 1
		}
		copy(grid[y][t.Col+1:], line)
	}
	// Selected tile gets a corner marker.
	for _, t := range tiles {
		if t.Item.Path != selected {
			continue
		}
		if t.Row < rows && t.Col < cols {
			grid[t.Row][t.Col] = '┏'
		}
		break
	}
	var b strings.Builder
	for _, r := range grid {
		b.WriteString(string(r))
		b.WriteString("\n")
	}
	return b.String()
}
