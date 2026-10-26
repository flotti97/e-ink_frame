// Package tag writes an Image's location onto its Slide, in a corner and a
// font of the user's choosing.
package tag

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"slices"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/flotti97/e-ink_frame/clock"
)

// The fonts, under the SIL Open Font License or the Apache License 2.0
// (font/OFL-*.txt, font/LICENSE-*.txt).
var (
	//go:embed font/CormorantGaramond-SemiBoldItalic.ttf
	cormorantTTF []byte
	//go:embed font/Caveat-wght.ttf
	caveatTTF []byte
	//go:embed font/PermanentMarker-Regular.ttf
	markerTTF []byte
	//go:embed font/PatrickHand-Regular.ttf
	patrickTTF []byte
	//go:embed font/HomemadeApple-Regular.ttf
	appleTTF []byte
)

// A Font a tag can be written in.
type Font struct {
	Key  string // as saved in the config
	Name string // as shown on the upload page
	ttf  []byte
	rule bool // a short rule leads the text, as in a gallery caption
}

// Fonts are the Fonts to choose from, the first the default.
var Fonts = []Font{
	{"cormorant", "Cormorant Garamond", cormorantTTF, true},
	{"caveat", "Caveat", caveatTTF, false},
	{"marker", "Permanent Marker", markerTTF, false},
	{"patrick", "Patrick Hand", patrickTTF, false},
	{"apple", "Homemade Apple", appleTTF, false},
}

// Positions are the corners a tag can go in.
const (
	TopLeft     = "top-left"
	TopRight    = "top-right"
	BottomLeft  = "bottom-left"
	BottomRight = "bottom-right"
)

// Positions lists the corners in the order the upload page offers them.
var Positions = []string{TopLeft, TopRight, BottomLeft, BottomRight}

// Limits of Settings.Size.
const (
	MinSize = 24
	MaxSize = 200
)

// Settings are how tags are written, on every Slide alike.
type Settings struct {
	On       bool
	Auto     bool   // tag new Images from their GPS position
	Position string // one of Positions
	Font     string // a Font's Key
	Size     int    // pixels per em
}

// Default is used for anything the config leaves out.
var Default = Settings{On: true, Auto: false, Position: TopLeft, Font: "cormorant", Size: 64}

// Validate reports settings tags can't be written with.
func (s Settings) Validate() error {
	if !slices.Contains(Positions, s.Position) {
		return fmt.Errorf("tag position %q unknown", s.Position)
	}
	if _, ok := fontByKey(s.Font); !ok {
		return fmt.Errorf("tag font %q unknown", s.Font)
	}
	if s.Size < MinSize || s.Size > MaxSize {
		return fmt.Errorf("tag size %d px out of range (%d..%d)", s.Size, MinSize, MaxSize)
	}
	return nil
}

// TTF is the font file.
func (f Font) TTF() []byte { return f.ttf }

func fontByKey(key string) (Font, bool) {
	i := slices.IndexFunc(Fonts, func(f Font) bool { return f.Key == key })
	if i < 0 {
		return Font{}, false
	}
	return Fonts[i], true
}

const (
	edge = 48 // between text and Slide edge, pixels
	halo = 3  // outline in the opposite color, pixels
)

var (
	facesMu sync.Mutex
	parsed  = map[string]*sfnt.Font{}
	faces   = map[Settings]font.Face{}
)

func face(f Font, size int) font.Face {
	facesMu.Lock()
	defer facesMu.Unlock()
	key := Settings{Font: f.Key, Size: size}
	if fc, ok := faces[key]; ok {
		return fc
	}
	sf, ok := parsed[f.Key]
	if !ok {
		var err error
		if sf, err = opentype.Parse(f.ttf); err != nil {
			panic(err) // embedded font
		}
		parsed[f.Key] = sf
	}
	fc, err := opentype.NewFace(sf, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic(err)
	}
	faces[key] = fc
	return fc
}

// Draw writes location onto s as set: black on light photos, white on dark
// ones, with a thin halo of the other color to keep it legible on busy
// ones. In a bottom corner it stays above y bottom, which keeps it clear of
// the Clock. Nothing is drawn while tags are off or location is "".
func Draw(s *image.Gray, location string, set Settings, bottom int) {
	f, ok := fontByKey(set.Font)
	if !set.On || location == "" || !ok {
		return
	}
	fc := face(f, set.Size)
	b := s.Bounds()
	m := fc.Metrics()
	adv := font.MeasureString(fc, location).Ceil()
	ruleW, gap := 0, 0
	if f.rule {
		ruleW, gap = set.Size*9/8, set.Size/3
	}
	w, h := ruleW+gap+adv, m.Ascent.Ceil()+m.Descent.Ceil()

	x, top := b.Min.X+edge, b.Min.Y+edge
	right := set.Position == TopRight || set.Position == BottomRight
	if right {
		x = b.Max.X - edge - w
	}
	if set.Position == BottomLeft || set.Position == BottomRight {
		top = min(b.Max.Y, bottom) - edge/2 - h
	}
	base := top + m.Ascent.Ceil()
	area := image.Rect(x, top, x+w, top+h).Intersect(b)
	ink := clock.Ink(s, area)
	back := color.Gray{0xFF - ink.Y}

	// The text and rule as a mask, then stamped in the halo color around
	// itself and in ink on top.
	mask := image.NewAlpha(b)
	textX, ruleX := x+ruleW+gap, x
	if right {
		textX, ruleX = x, x+adv+gap
	}
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: fc, Dot: fixed.P(textX, base)}
	d.DrawString(location)
	if ruleW > 0 {
		ruleY := base - m.Ascent.Ceil()/3
		draw.Draw(mask, image.Rect(ruleX, ruleY-1, ruleX+ruleW, ruleY+2), image.Opaque, image.Point{}, draw.Src)
	}

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
