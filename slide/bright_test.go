package slide

import (
	"image"
	"image/color"
	"testing"
)

func TestBrightenLiftsMidtonesKeepsEnds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	img.Set(0, 0, color.Gray{0})
	img.Set(1, 0, color.Gray{64})
	img.Set(2, 0, color.Gray{255})
	Brighten(img, 50)
	if got := img.RGBAAt(0, 0).R; got != 0 {
		t.Errorf("black became %d", got)
	}
	if got := img.RGBAAt(1, 0).R; got <= 100 {
		t.Errorf("dark gray 64 only became %d", got)
	}
	if got := img.RGBAAt(2, 0).R; got != 255 {
		t.Errorf("white became %d", got)
	}
}

func TestBrightenZeroIsIdentity(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Gray{77})
	Brighten(img, 0)
	if got := img.RGBAAt(0, 0).R; got != 77 {
		t.Errorf("got %d", got)
	}
}
