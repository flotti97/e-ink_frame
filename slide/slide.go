// Package slide turns an Image into a Slide: cropped to fill the panel with a
// saliency-based smart crop, then toned to the panel's 16 gray levels.
package slide

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
)

// Panel resolution of the Waveshare 10.3" e-Paper HAT, landscape.
const (
	Width  = 1872
	Height = 1404
)

// ErrUnsupported is returned for files that are neither JPEG nor PNG.
var ErrUnsupported = errors.New("unsupported image format (want JPEG or PNG)")

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
)

// Decode reads a JPEG or PNG Image, turning JPEGs upright according to
// their EXIF Orientation. Other formats yield ErrUnsupported;
// unreadable JPEG or PNG data yields an error mentioning "corrupt".
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	head := data[:min(len(data), len(pngMagic))]

	var decode func(io.Reader) (image.Image, error)
	var format string
	switch {
	case bytes.HasPrefix(head, jpegMagic):
		decode, format = jpeg.Decode, "JPEG"
	case bytes.HasPrefix(head, pngMagic):
		decode, format = png.Decode, "PNG"
	default:
		return nil, ErrUnsupported
	}

	img, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", format, err)
	}
	if format == "JPEG" {
		img = orient(img, exifOrientation(data))
	}
	return img, nil
}

// Render produces the Slide for img: a Width×Height image in 16 gray levels.
func Render(img image.Image) *image.Gray {
	crop := Crop(img, Width, Height)
	scaled := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, crop, draw.Src, nil)
	return Tone(scaled)
}
