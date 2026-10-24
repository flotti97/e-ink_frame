package clock

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// A glyph is a Hershey character: pen strokes in font units, y down, with
// the baseline near 9, and the horizontal extent it advances over.
type glyph struct {
	left, right int
	strokes     [][]point
}

type point struct{ x, y float64 }

// parseJHF reads a Hershey font in James Hurt's format, whose records hold
// the printable ASCII characters in order from space. A record is a 5-digit
// number, a 3-digit vertex count, then that many coordinate pairs as
// letters offset from 'R'; " R" lifts the pen. The first pair is the left
// and right extent. Records may wrap over lines.
func parseJHF(data string) (map[rune]glyph, error) {
	data = strings.ReplaceAll(data, "\n", "")
	font := map[rune]glyph{}
	for c := ' '; len(strings.TrimSpace(data)) > 0; c++ {
		if len(data) < 8 {
			return nil, fmt.Errorf("record for %q cut short", c)
		}
		n, err := strconv.Atoi(strings.TrimSpace(data[5:8]))
		if err != nil || n < 1 || len(data) < 8+2*n {
			return nil, fmt.Errorf("record for %q: bad vertex count %q", c, data[5:8])
		}
		v := data[8 : 8+2*n]
		data = data[8+2*n:]
		g := glyph{left: int(v[0]) - 'R', right: int(v[1]) - 'R'}
		var cur []point
		for i := 2; i < len(v); i += 2 {
			if v[i:i+2] == " R" {
				if len(cur) > 0 {
					g.strokes = append(g.strokes, cur)
				}
				cur = nil
				continue
			}
			cur = append(cur, point{float64(int(v[i]) - 'R'), float64(int(v[i+1]) - 'R')})
		}
		if len(cur) > 0 {
			g.strokes = append(g.strokes, cur)
		}
		font[c] = g
	}
	return font, nil
}
