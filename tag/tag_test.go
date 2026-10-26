package tag

import (
	"image"
	"testing"
)

func TestDrawInksTopLeftOnly(t *testing.T) {
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	for i := range s.Pix {
		s.Pix[i] = 0xFF
	}
	Draw(s, "Hallstatt, Austria")
	var dark image.Rectangle
	for y := range 1404 {
		for x := range 1872 {
			v := s.GrayAt(x, y).Y
			if v%17 != 0 {
				t.Fatalf("gray %d at %d,%d is not a panel level", v, x, y)
			}
			if v < 0x80 {
				dark = dark.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if dark.Empty() || dark.Max.X > 1872/2 || dark.Max.Y > 1404/4 {
		t.Fatalf("ink at %v, want it in the top left", dark)
	}
}

func TestDrawEmptyDrawsNothing(t *testing.T) {
	s := image.NewGray(image.Rect(0, 0, 100, 100))
	Draw(s, "")
	for _, v := range s.Pix {
		if v != 0 {
			t.Fatal("empty location changed the Slide")
		}
	}
}
