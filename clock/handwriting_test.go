package clock

import (
	"image"
	"image/color"
	"testing"
)

var bounds = image.Rect(0, 0, 1872, 1404)

func TestStepsStayInsideTheArea(t *testing.T) {
	area := Area(bounds)
	if area.Min.X%4 != 0 || area.Dx()%4 != 0 || !area.In(bounds) {
		t.Fatalf("area %v not aligned or outside the Slide", area)
	}
	for m := 0; m < 60; m += 5 {
		s := image.NewGray(bounds)
		steps := Steps(at(10, m), bounds, 20)
		if len(steps) < 10 {
			t.Errorf("10:%02d: only %d steps", m, len(steps))
		}
		for _, st := range steps {
			r := st.Draw(s, color.Gray{0xFF})
			if r.Min.X%4 != 0 || r.Dx()%4 != 0 || !r.In(area) {
				t.Fatalf("10:%02d: step area %v not aligned or outside %v", m, r, area)
			}
		}
	}
}

func TestStepDrawReportsWhereItInked(t *testing.T) {
	s := image.NewGray(bounds)
	r := Step{{100, 100}, {140, 110}}.Draw(s, color.Gray{0xFF})
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			if s.GrayAt(x, y).Y != 0 && !image.Pt(x, y).In(r) {
				t.Fatalf("ink at %d,%d outside %v", x, y, r)
			}
		}
	}
	if s.GrayAt(120, 105).Y != 0xFF {
		t.Error("no ink on the line")
	}
}

func TestInkContrastsWithTheBackground(t *testing.T) {
	s := image.NewGray(image.Rect(0, 0, 8, 8))
	if Ink(s, s.Bounds()).Y != 0xFF {
		t.Error("dark background: want white ink")
	}
	for i := range s.Pix {
		s.Pix[i] = 0xC0
	}
	if Ink(s, s.Bounds()).Y != 0 {
		t.Error("light background: want black ink")
	}
}

func TestBandHoldsTheAreaAlongTheBottom(t *testing.T) {
	band, area := Band(bounds), Area(bounds)
	if !area.In(band) || band.Dx() != bounds.Dx() || band.Max.Y != bounds.Max.Y {
		t.Errorf("band %v for area %v", band, area)
	}
}

func TestShorterStepsMakeMoreSteps(t *testing.T) {
	if a, b := len(Steps(at(10, 0), bounds, 20)), len(Steps(at(10, 0), bounds, 60)); a <= b {
		t.Errorf("%d steps of 20 px, %d of 60 px", a, b)
	}
}
