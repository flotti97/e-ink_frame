package tag

import (
	"image"
	"testing"
)

func white() *image.Gray {
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	for i := range s.Pix {
		s.Pix[i] = 0xFF
	}
	return s
}

// inked is the area of s with dark pixels, failing on any gray the panel
// can't show.
func inked(t *testing.T, s *image.Gray) image.Rectangle {
	t.Helper()
	var dark image.Rectangle
	b := s.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := s.GrayAt(x, y).Y
			if v%17 != 0 {
				t.Fatalf("gray %d at %d,%d is not a panel level", v, x, y)
			}
			if v < 0x80 {
				dark = dark.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return dark
}

func TestDrawEveryFontInEveryCorner(t *testing.T) {
	for _, f := range Fonts {
		for _, pos := range Positions {
			s := white()
			set := Settings{On: true, Position: pos, Font: f.Key, Size: 64}
			Draw(s, "Hallstatt, Austria", set, 1200)
			dark := inked(t, s)
			if dark.Empty() {
				t.Fatalf("%s %s: nothing drawn", f.Key, pos)
			}
			left := pos == TopLeft || pos == BottomLeft
			top := pos == TopLeft || pos == TopRight
			if left != (dark.Max.X < 1872/2+200) || top != (dark.Max.Y < 1404/2) {
				t.Errorf("%s %s: ink at %v", f.Key, pos, dark)
			}
			if !top && dark.Max.Y > 1200 {
				t.Errorf("%s %s: ink at %v reaches below the Clock's band", f.Key, pos, dark)
			}
		}
	}
}

func TestDrawNothingWhenOffOrEmpty(t *testing.T) {
	for _, c := range []struct {
		loc string
		on  bool
	}{{"", true}, {"Lisbon", false}} {
		s := white()
		Draw(s, c.loc, Settings{On: c.on, Position: TopLeft, Font: "caveat", Size: 64}, 1404)
		if !inked(t, s).Empty() {
			t.Errorf("%+v changed the Slide", c)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Default.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Settings{
		{Position: "middle", Font: "caveat", Size: 64},
		{Position: TopLeft, Font: "comic", Size: 64},
		{Position: TopLeft, Font: "caveat", Size: MaxSize + 1},
	} {
		if bad.Validate() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
}
