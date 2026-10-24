package slide

import (
	"image"
	"image/color"
	"testing"
)

// flatWithDetail returns a flat gray image with a high-contrast checkerboard
// patch at detail, standing in for the subject of a photo.
func flatWithDetail(w, h int, detail image.Rectangle) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{128, 128, 128, 255}
			if (image.Point{x, y}).In(detail) {
				if (x/4+y/4)%2 == 0 {
					c = color.RGBA{0, 0, 0, 255}
				} else {
					c = color.RGBA{255, 255, 255, 255}
				}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func TestCropKeepsPanelAspectAndFills(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"landscape 3:2", 1500, 1000, 1333, 1000},
		{"portrait", 1000, 1500, 1000, 750},
		{"16:9", 1600, 900, 1200, 900},
		{"exact 4:3", 1872, 1404, 1872, 1404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
			r := Crop(img, Width, Height)
			if r.Dx() != c.wantW || r.Dy() != c.wantH {
				t.Errorf("crop is %dx%d, want %dx%d", r.Dx(), r.Dy(), c.wantW, c.wantH)
			}
			if !r.In(img.Bounds()) {
				t.Errorf("crop %v leaves the image %v", r, img.Bounds())
			}
		})
	}
}

func TestCropFollowsSalientDetailHorizontally(t *testing.T) {
	// 16:9 with the subject at the far right: the 4:3 window must slide right.
	img := flatWithDetail(1600, 900, image.Rect(1450, 300, 1580, 600))
	r := Crop(img, Width, Height)
	if !image.Rect(1450, 300, 1580, 600).In(r) {
		t.Errorf("crop %v cuts off the subject at the right edge", r)
	}
}

func TestCropFollowsSalientDetailVertically(t *testing.T) {
	// Portrait with the subject near the top.
	img := flatWithDetail(1000, 1500, image.Rect(300, 40, 700, 300))
	r := Crop(img, Width, Height)
	if !image.Rect(300, 40, 700, 300).In(r) {
		t.Errorf("crop %v cuts off the subject at the top", r)
	}
}

func TestCropCentersWhenNothingStandsOut(t *testing.T) {
	img := flatWithDetail(1600, 900, image.Rectangle{})
	r := Crop(img, Width, Height)
	if r.Min.X != 200 {
		t.Errorf("flat image crop starts at x=%d, want centered at 200", r.Min.X)
	}
}

func TestCropRespectsNonZeroBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(100, 50, 1700, 950))
	r := Crop(img, Width, Height)
	if !r.In(img.Bounds()) {
		t.Errorf("crop %v leaves the image %v", r, img.Bounds())
	}
}

func TestCropNeverReturnsAnEmptyRectangle(t *testing.T) {
	for _, size := range []image.Point{{1, 1}, {2000, 1}, {1, 2000}} {
		r := Crop(image.NewRGBA(image.Rect(0, 0, size.X, size.Y)), Width, Height)
		if r.Empty() {
			t.Errorf("%v Image gave an empty crop", size)
		}
	}
}
