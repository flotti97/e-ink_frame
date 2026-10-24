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

func TestToneDithersFlatAreasToTheirMeanGray(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 40, 40))
	for i := range src.Pix {
		src.Pix[i] = 100 // between levels 85 and 102
	}
	got := Tone(src)

	sum, levels := 0, map[uint8]bool{}
	for _, v := range got.Pix {
		sum += int(v)
		levels[v] = true
	}
	if len(levels) < 2 {
		t.Fatal("flat in-between gray produced a single level; not dithered")
	}
	if mean := float64(sum) / float64(len(got.Pix)); mean < 98 || mean > 102 {
		t.Errorf("dithered mean = %.1f, want about 100", mean)
	}
}
