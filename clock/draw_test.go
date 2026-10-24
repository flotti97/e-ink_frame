package clock

import (
	"image"
	"testing"
)

func TestDrawStaysInsideAnAlignedBox(t *testing.T) {
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	for i := range s.Pix {
		s.Pix[i] = 0x80
	}
	box := Draw(s, at(14, 20))
	if box.Min.X%4 != 0 || box.Dx()%4 != 0 || !box.In(s.Bounds()) {
		t.Fatalf("box %v not aligned or outside the Slide", box)
	}
	var ink bool
	for y := range s.Bounds().Dy() {
		for x := range s.Bounds().Dx() {
			v := s.GrayAt(x, y).Y
			in := image.Pt(x, y).In(box)
			if !in && v != 0x80 {
				t.Fatalf("pixel %d,%d outside the box changed", x, y)
			}
			ink = ink || in && v < 0x40
		}
	}
	if !ink {
		t.Error("no text drawn")
	}
}

func TestBoxIsTheSameForEveryPhrase(t *testing.T) {
	a := Draw(image.NewGray(image.Rect(0, 0, 1872, 1404)), at(12, 0))
	b := Draw(image.NewGray(image.Rect(0, 0, 1872, 1404)), at(11, 25))
	if a != b {
		t.Errorf("box %v for zwölf, %v for fünf vor halb zwölf", a, b)
	}
}
