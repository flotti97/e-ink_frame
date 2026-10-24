package slide

import (
	"bytes"
	"encoding/binary"
	"image"
)

// exifOrientation returns the EXIF Orientation (1–8) of a JPEG, or 1 when the
// file has none or its EXIF data can't be read.
func exifOrientation(data []byte) int {
	for i := 2; i+4 <= len(data) && data[i] == 0xFF; {
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // image data starts; no more metadata
			break
		}
		end := i + 2 + int(binary.BigEndian.Uint16(data[i+2:]))
		if end > len(data) {
			break
		}
		if marker == 0xE1 && bytes.HasPrefix(data[i+4:end], []byte("Exif\x00\x00")) {
			return tiffOrientation(data[i+10 : end])
		}
		i = end
	}
	return 1
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int64(order.Uint32(tiff[4:]))
	if ifd+2 > int64(len(tiff)) {
		return 1
	}
	n := int(order.Uint16(tiff[ifd:]))
	for e := 0; e < n; e++ {
		at := ifd + 2 + int64(e)*12
		if at+12 > int64(len(tiff)) {
			break
		}
		if order.Uint16(tiff[at:]) == 0x0112 {
			if v := int(order.Uint16(tiff[at+8:])); v >= 1 && v <= 8 {
				return v
			}
			break
		}
	}
	return 1
}

// orient returns img turned upright according to an EXIF Orientation value.
func orient(img image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	size := image.Pt(w, h)
	if orientation >= 5 {
		size = image.Pt(h, w)
	}
	out := image.NewRGBA(image.Rectangle{Max: size})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
