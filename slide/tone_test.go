package slide

import (
	"image"
	"image/color"
	"testing"
)

func TestToneUsesOnlySixteenGrayLevels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 256, 3))
	for x := 0; x < 256; x++ {
		v := uint8(x)
		src.Set(x, 0, color.RGBA{v, v, v, 255})
		src.Set(x, 1, color.RGBA{v, 0, 255 - v, 255})
		src.Set(x, 2, color.RGBA{0, v, v / 2, 255})
	}

	got := Tone(src)

	levels := map[uint8]bool{}
	for _, v := range got.Pix {
		if v%17 != 0 {
			t.Fatalf("gray value %d is not one of the 16 panel levels", v)
		}
		levels[v] = true
	}
	if len(levels) != 16 {
		t.Errorf("gray ramp produced %d levels, want all 16", len(levels))
	}
}

func TestToneMapsExtremesAndRoundsToNearestLevel(t *testing.T) {
	cases := []struct {
		in   uint8
		want uint8
	}{
		{0, 0}, {8, 0}, {9, 17}, {255, 255}, {246, 238}, {247, 255}, {128, 136},
	}
	for _, c := range cases {
		src := image.NewGray(image.Rect(0, 0, 1, 1))
		src.Pix[0] = c.in
		if got := Tone(src).Pix[0]; got != c.want {
			t.Errorf("Tone(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestToneHasNoDithering(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 20, 20))
	for i := range src.Pix {
		src.Pix[i] = 100
	}
	got := Tone(src)
	for _, v := range got.Pix {
		if v != got.Pix[0] {
			t.Fatal("flat input produced varying output; looks dithered")
		}
	}
}
