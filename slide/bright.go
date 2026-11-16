package slide

import (
	"image"
	"math"
)

// Limits of a Brightness.
const (
	MinBrightness = -100
	MaxBrightness = 100
)

// Brighten lifts (or, below 0, darkens) the midtones and shadows of img in
// place with a gamma curve, so a dark photo reads on the panel without
// losing its whites or blacks. brightness 0 leaves img as it is; 100 is a
// gamma of 1/4, -100 a gamma of 4.
func Brighten(img *image.RGBA, brightness int) {
	brightness = min(max(brightness, MinBrightness), MaxBrightness)
	if brightness == 0 {
		return
	}
	g := math.Pow(2, -float64(brightness)/50)
	var lut [256]uint8
	for i := range lut {
		lut[i] = uint8(math.Round(255 * math.Pow(float64(i)/255, g)))
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = lut[img.Pix[i]]
		img.Pix[i+1] = lut[img.Pix[i+1]]
		img.Pix[i+2] = lut[img.Pix[i+2]]
	}
}
