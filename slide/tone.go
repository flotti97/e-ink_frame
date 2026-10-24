package slide

import (
	"image"
	"image/color"
)

// Levels is the number of gray levels the panel can show.
const Levels = 16

const levelStep = 255 / (Levels - 1) // 17

// Tone converts img to grayscale and reduces it to the panel's 16 gray levels
// with Floyd–Steinberg dithering, so smooth gradients such as skies don't band.
func Tone(img image.Image) *image.Gray {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))

	// Two rows of gray values plus diffused error, in sixteenths so no error
	// is lost to integer division, with a pixel of padding on each side so
	// neighbours never need bounds checks.
	cur := make([]int, w+2)
	next := make([]int, w+2)
	for x := 0; x < w; x++ {
		cur[x+1] = 16 * gray(img, b.Min.X+x, b.Min.Y)
	}
	for y := 0; y < h; y++ {
		for x := range next {
			next[x] = 0
		}
		if y+1 < h {
			for x := 0; x < w; x++ {
				next[x+1] = 16 * gray(img, b.Min.X+x, b.Min.Y+y+1)
			}
		}
		for x := 0; x < w; x++ {
			v := cur[x+1]
			q := quantize((v + 8) / 16)
			out.Pix[y*out.Stride+x] = uint8(q)
			e := v - 16*q
			cur[x+2] += e * 7 / 16
			next[x] += e * 3 / 16
			next[x+1] += e * 5 / 16
			next[x+2] += e / 16
		}
		cur, next = next, cur
	}
	return out
}

func gray(img image.Image, x, y int) int {
	return int(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y)
}

// quantize rounds v, which may lie outside 0..255 after error diffusion, to
// the nearest panel level.
func quantize(v int) int {
	v = min(max(v, 0), 255)
	return (v + levelStep/2) / levelStep * levelStep
}
