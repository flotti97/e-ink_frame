package slide

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

var (
	red  = color.RGBA{255, 0, 0, 255}
	blue = color.RGBA{0, 0, 255, 255}
)

// redBlueJPEG encodes a 32×16 JPEG, left half red and right half blue, with an
// EXIF Orientation tag (none when orientation is 0).
func redBlueJPEG(t *testing.T, orientation uint16, order binary.AppendByteOrder) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			if x < 16 {
				img.Set(x, y, red)
			} else {
				img.Set(x, y, blue)
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	if orientation == 0 {
		return buf.Bytes()
	}

	tiff := []byte("MM\x00\x2a")
	if order == binary.LittleEndian {
		tiff = []byte("II\x2a\x00")
	}
	tiff = order.AppendUint32(tiff, 8)
	tiff = order.AppendUint16(tiff, 1)      // one IFD entry
	tiff = order.AppendUint16(tiff, 0x0112) // Orientation
	tiff = order.AppendUint16(tiff, 3)      // SHORT
	tiff = order.AppendUint32(tiff, 1)
	tiff = order.AppendUint16(tiff, orientation)
	tiff = order.AppendUint16(tiff, 0)
	tiff = order.AppendUint32(tiff, 0) // no next IFD

	payload := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(payload)+2))
	app1 = append(app1, payload...)

	data := buf.Bytes()
	out := append([]byte{}, data[:2]...) // SOI
	out = append(out, app1...)
	return append(out, data[2:]...)
}

func isColor(c color.Color, want color.RGBA) bool {
	r, g, b, _ := c.RGBA()
	near := func(got uint32, want uint8) bool {
		d := int(got>>8) - int(want)
		return d > -40 && d < 40
	}
	return near(r, want.R) && near(g, want.G) && near(b, want.B)
}

func TestDecodeAppliesEXIFOrientation(t *testing.T) {
	// Where the red half and the blue half end up, sampled at their centers.
	cases := []struct {
		orientation   uint16
		size          image.Point
		redAt, blueAt image.Point
	}{
		{0, image.Pt(32, 16), image.Pt(8, 8), image.Pt(24, 8)},
		{1, image.Pt(32, 16), image.Pt(8, 8), image.Pt(24, 8)},
		{2, image.Pt(32, 16), image.Pt(24, 8), image.Pt(8, 8)},
		{3, image.Pt(32, 16), image.Pt(24, 8), image.Pt(8, 8)},
		{4, image.Pt(32, 16), image.Pt(8, 8), image.Pt(24, 8)},
		{5, image.Pt(16, 32), image.Pt(8, 8), image.Pt(8, 24)},
		{6, image.Pt(16, 32), image.Pt(8, 8), image.Pt(8, 24)},
		{7, image.Pt(16, 32), image.Pt(8, 24), image.Pt(8, 8)},
		{8, image.Pt(16, 32), image.Pt(8, 24), image.Pt(8, 8)},
	}
	for _, order := range []binary.AppendByteOrder{binary.BigEndian, binary.LittleEndian} {
		for _, c := range cases {
			img, err := Decode(bytes.NewReader(redBlueJPEG(t, c.orientation, order)))
			if err != nil {
				t.Fatalf("orientation %d (%v): %v", c.orientation, order, err)
			}
			if got := img.Bounds().Size(); got != c.size {
				t.Errorf("orientation %d (%v): size %v, want %v", c.orientation, order, got, c.size)
				continue
			}
			o := img.Bounds().Min
			if !isColor(img.At(o.X+c.redAt.X, o.Y+c.redAt.Y), red) ||
				!isColor(img.At(o.X+c.blueAt.X, o.Y+c.blueAt.Y), blue) {
				t.Errorf("orientation %d (%v): red/blue halves in the wrong place", c.orientation, order)
			}
		}
	}
}

func TestDecodeIgnoresMalformedEXIF(t *testing.T) {
	data := redBlueJPEG(t, 6, binary.BigEndian)
	// Point IFD0 far past the end of the segment.
	copy(data[2+2+2+6+4:], []byte{0x7F, 0xFF, 0xFF, 0xFF})
	img, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Size() != image.Pt(32, 16) {
		t.Errorf("malformed EXIF changed the size to %v", img.Bounds().Size())
	}
}
