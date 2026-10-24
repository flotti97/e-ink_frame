package clock

import (
	"image"
	"image/draw"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	fontSize = 64 // pixels
	padding  = 24 // between text and box edge
	margin   = 40 // between box and Slide edge
)

var (
	faceOnce sync.Once
	face     font.Face
	boxSize  image.Point // fits the widest Phrase, width a multiple of 4
)

func load() {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(err) // embedded font
	}
	face, err = opentype.NewFace(f, &opentype.FaceOptions{Size: fontSize, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	var widest fixed.Int26_6
	for m := 0; m < 12*60; m += 5 {
		widest = max(widest, font.MeasureString(face, Phrase(time.Date(0, 1, 1, 0, m, 0, 0, time.UTC))))
	}
	w := (widest.Ceil() + 2*padding + 3) &^ 3
	boxSize = image.Pt(w, face.Metrics().Height.Ceil()+2*padding)
}

// Box is where Draw puts the clock on a Slide with bounds b: the bottom
// right corner, with its left edge and width multiples of 4 so it can be
// refreshed on its own.
func Box(b image.Rectangle) image.Rectangle {
	faceOnce.Do(load)
	x := (b.Max.X - margin - boxSize.X) &^ 3
	y := b.Max.Y - margin - boxSize.Y
	return image.Rect(x, y, x+boxSize.X, y+boxSize.Y)
}

// Draw puts the time t in words on s, right-aligned in a white box, and
// returns the box.
func Draw(s *image.Gray, t time.Time) image.Rectangle {
	box := Box(s.Bounds())
	draw.Draw(s, box, image.White, image.Point{}, draw.Src)
	text := Phrase(t)
	m := face.Metrics()
	d := font.Drawer{Dst: s, Src: image.Black, Face: face}
	d.Dot = fixed.P(box.Max.X-padding, box.Min.Y+padding+m.Ascent.Ceil())
	d.Dot.X -= d.MeasureString(text)
	d.DrawString(text)
	return box
}
