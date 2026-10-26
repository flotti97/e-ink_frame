// Package tag writes an Image's location onto its Slide, small and in an
// italic serif, in the top left corner, away from the Clock.
package tag

import (
	_ "embed"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/flotti97/e-ink_frame/clock"
)

// Cormorant Garamond SemiBold Italic, under the SIL Open Font License
// (font/OFL.txt).
//
//go:embed font/CormorantGaramond-SemiBoldItalic.ttf
var ttf []byte

const (
	size  = 64 // pixels per em
	edge  = 48 // between text and Slide edge, pixels
	halo  = 3  // outline in the opposite color, pixels
	ruleW = 72 // length of the rule before the text, pixels
)

var face = func() font.Face {
	f, err := opentype.Parse(ttf)
	if err != nil {
		panic(err) // embedded font
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic(err)
	}
	return fc
}()

// Draw writes location onto s: "— location", black on light photos, white
// on dark ones, with a thin halo of the other color to keep it legible on
// busy ones. An empty location draws nothing.
func Draw(s *image.Gray, location string) {
	if location == "" {
		return
	}
	b := s.Bounds()
	m := face.Metrics()
	adv := font.MeasureString(face, location).Ceil()
	gap := size / 3
	x := b.Min.X + edge
	base := b.Min.Y + edge + m.Ascent.Ceil()
	area := image.Rect(x, b.Min.Y+edge, x+ruleW+gap+adv, base+m.Descent.Ceil()).Intersect(b)
	ink := clock.Ink(s, area)
	back := color.Gray{0xFF - ink.Y}

	// The text and rule as a mask, then stamped in the halo color around
	// itself and in ink on top.
	mask := image.NewAlpha(b)
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: face, Dot: fixed.P(x+ruleW+gap, base)}
	d.DrawString(location)
	ruleY := base - m.Ascent.Ceil()/3
	draw.Draw(mask, image.Rect(x, ruleY-1, x+ruleW, ruleY+2), image.Opaque, image.Point{}, draw.Src)

	r := area.Inset(-halo)
	for dy := -halo; dy <= halo; dy++ {
		for dx := -halo; dx <= halo; dx++ {
			if dx*dx+dy*dy > halo*halo {
				continue
			}
			draw.DrawMask(s, r, image.NewUniform(back), image.Point{}, mask, r.Min.Add(image.Pt(dx, dy)), draw.Over)
		}
	}
	draw.DrawMask(s, r, image.NewUniform(ink), image.Point{}, mask, r.Min, draw.Over)
	quantize(s, r)
}

// quantize snaps r of s to the panel's 16 gray levels, which the font's
// anti-aliased edges leave.
func quantize(s *image.Gray, r image.Rectangle) {
	r = r.Intersect(s.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := s.PixOffset(x, y)
			s.Pix[i] = uint8((int(s.Pix[i]) + 8) / 17 * 17)
		}
	}
}
