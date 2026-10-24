package slide

import (
	"image"
	"image/color"
)

// Levels is the number of gray levels the panel can show.
const Levels = 16

const levelStep = 255 / (Levels - 1) // 17

// Tone converts img to grayscale and rounds every pixel to the nearest of the
// panel's 16 gray levels. There is deliberately no dithering.
func Tone(img image.Image) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y
			out.Pix[(y-b.Min.Y)*out.Stride+(x-b.Min.X)] = quantize(v)
		}
	}
	return out
}

func quantize(v uint8) uint8 {
	return uint8((int(v)+levelStep/2)/levelStep) * levelStep
}
