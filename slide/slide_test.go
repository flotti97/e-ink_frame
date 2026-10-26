package slide

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestRenderProducesPanelSizedSlide(t *testing.T) {
	for _, size := range []image.Point{{1500, 1000}, {1000, 1500}, {1600, 900}, {300, 200}} {
		got := Render(flatWithDetail(size.X, size.Y, image.Rect(10, 10, 60, 60)))
		if got.Bounds() != image.Rect(0, 0, Width, Height) {
			t.Errorf("%v Image gave a %v Slide", size, got.Bounds())
		}
		for _, v := range got.Pix {
			if v%levelStep != 0 {
				t.Fatalf("%v Image: Slide has off-panel gray %d", size, v)
			}
		}
	}
}

func TestDecodeAcceptsJPEGAndPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	var j, p bytes.Buffer
	if err := jpeg.Encode(&j, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&p, img); err != nil {
		t.Fatal(err)
	}
	for name, buf := range map[string]*bytes.Buffer{"jpeg": &j, "png": &p} {
		if _, err := Decode(buf); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDecodeRejectsUnsupportedAndCorruptFiles(t *testing.T) {
	var p bytes.Buffer
	png.Encode(&p, image.NewRGBA(image.Rect(0, 0, 8, 6)))
	truncated := p.Bytes()[:p.Len()/2]

	cases := map[string]struct {
		data []byte
		want string
	}{
		"gif":       {[]byte("GIF89a\x01\x00\x01\x00"), "unsupported"},
		"text":      {[]byte("hello"), "unsupported"},
		"empty":     {nil, "unsupported"},
		"truncated": {truncated, "corrupt"},
	}
	for name, c := range cases {
		_, err := Decode(bytes.NewReader(c.data))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
}

func TestCheckAcceptsRenderedSlidesOnly(t *testing.T) {
	encode := func(img image.Image) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	good := Render(image.NewRGBA(image.Rect(0, 0, 300, 200)))
	if err := Check(encode(good)); err != nil {
		t.Errorf("rendered Slide rejected: %v", err)
	}
	off := image.NewGray(good.Bounds())
	off.Pix[0] = 3
	for name, data := range map[string][]byte{
		"not a PNG":  []byte("hello"),
		"wrong size": encode(image.NewGray(image.Rect(0, 0, 10, 10))),
		"colour":     encode(image.NewRGBA(good.Bounds())),
		"off level":  encode(off),
	} {
		if Check(data) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
