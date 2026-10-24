package clock

import (
	_ "embed"
	"image"
	"image/color"
	"math"
	"time"
)

//go:embed hershey/scripts.jhf
var scriptJHF string

var script = mustParse(scriptJHF)

func mustParse(data string) map[rune]glyph {
	f, err := parseJHF(data)
	if err != nil {
		panic(err) // embedded font
	}
	return f
}

const (
	scale    = 4.0 // pixels per font unit
	penWidth = 7   // pixels
	stepLen  = 20  // pixels of pen travel per animation step
	edge     = 48  // between text and Slide edge, pixels
)

// A Step is a short piece of one pen stroke, the ink one animation frame
// adds.
type Step []point

// umlauts are written as their base letter and two dots above it.
var umlauts = map[rune]rune{'ä': 'a', 'ö': 'o', 'ü': 'u'}

// strokes lays out text in the script font, right-aligned in the bottom
// right corner of a Slide with bounds b, in pixels.
func strokes(text string, b image.Rectangle) [][]point {
	var width int
	for _, c := range text {
		if u, ok := umlauts[c]; ok {
			c = u
		}
		g := script[c]
		width += g.right - g.left
	}
	x := float64(b.Max.X-edge) - float64(width)*scale
	base := float64(b.Max.Y-edge) - 16*scale // room for descenders
	var out [][]point
	for _, c := range text {
		u, umlaut := umlauts[c]
		if umlaut {
			c = u
		}
		g := script[c]
		for _, s := range g.strokes {
			ps := make([]point, len(s))
			for i, p := range s {
				ps[i] = point{x + float64(p.x-float64(g.left))*scale, base + (p.y-9)*scale}
			}
			out = append(out, ps)
		}
		if umlaut {
			// Over the letter's body, which starts at x 0, a little higher
			// than the dot on an i.
			mid := x + float64(-g.left)*scale
			for _, dx := range []float64{-1, 3} {
				d := point{mid + dx*scale, base - 15*scale}
				out = append(out, []point{d, {d.x + 0.5*scale, d.y - 0.5*scale}})
			}
		}
		x += float64(g.right-g.left) * scale
	}
	return out
}

// Steps is the pen's path writing the time t on a Slide with bounds b, cut
// into animation steps of about stepLen pixels of travel each.
func Steps(t time.Time, b image.Rectangle) []Step {
	var out []Step
	for _, s := range strokes(Phrase(t), b) {
		cur := Step{s[0]}
		var run float64
		for _, p := range s[1:] {
			last := cur[len(cur)-1]
			run += math.Hypot(p.x-last.x, p.y-last.y)
			cur = append(cur, p)
			if run >= stepLen {
				out = append(out, cur)
				cur, run = Step{p}, 0
			}
		}
		if len(cur) > 1 || len(s) == 1 {
			out = append(out, cur)
		}
	}
	return out
}

// Draw inks st onto s and returns the area it touched, with its left edge
// and width multiples of 4 so it can be refreshed on its own.
func (st Step) Draw(s *image.Gray, ink color.Gray) image.Rectangle {
	var r image.Rectangle
	for i := range st {
		a, b := st[i], st[min(i+1, len(st)-1)]
		r = r.Union(line(s, a, b, ink))
	}
	return align(r).Intersect(s.Bounds())
}

// line draws a round-ended line of penWidth from a to b.
func line(s *image.Gray, a, b point, ink color.Gray) image.Rectangle {
	const rad = penWidth / 2.0
	r := image.Rect(
		int(math.Floor(min(a.x, b.x)-rad)), int(math.Floor(min(a.y, b.y)-rad)),
		int(math.Ceil(max(a.x, b.x)+rad))+1, int(math.Ceil(max(a.y, b.y)+rad))+1,
	).Intersect(s.Bounds())
	dx, dy := b.x-a.x, b.y-a.y
	l2 := dx*dx + dy*dy
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if l2 > 0 {
				t = max(0, min(1, ((px-a.x)*dx+(py-a.y)*dy)/l2))
			}
			if math.Hypot(px-a.x-t*dx, py-a.y-t*dy) <= rad {
				s.SetGray(x, y, ink)
			}
		}
	}
	return r
}

func align(r image.Rectangle) image.Rectangle {
	r.Min.X &^= 3
	r.Max.X = (r.Max.X + 3) &^ 3
	return r
}

// Ink is the color that stands out best on area r of s: black on a light
// background, white on a dark one. The quick Updates only reach these two.
func Ink(s *image.Gray, r image.Rectangle) color.Gray {
	var sum, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sum += int(s.Pix[s.PixOffset(x, y)])
			n++
		}
	}
	if n > 0 && sum/n < 0x80 {
		return color.Gray{0xFF}
	}
	return color.Gray{0}
}

// Area is where any phrase can be written on a Slide with bounds b, aligned
// so it can be refreshed on its own.
func Area(b image.Rectangle) image.Rectangle {
	var r image.Rectangle
	for m := 0; m < 12*60; m += 5 {
		for _, s := range strokes(Phrase(time.Date(0, 1, 1, 0, m, 0, 0, time.UTC)), b) {
			for _, p := range s {
				r = r.Union(image.Rect(int(p.x), int(p.y), int(p.x)+1, int(p.y)+1))
			}
		}
	}
	return align(r.Inset(-penWidth)).Intersect(b)
}
