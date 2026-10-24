// Command panel puts one Slide on the real panel with a full Refresh, then
// puts the panel to sleep. It runs on the Pi.
//
//	panel -vcom -1.58 <slide.png>
//	panel -vcom -1.58 -test-card
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/flotti97/e-ink_frame/it8951"
	"github.com/flotti97/e-ink_frame/slide"
)

func main() {
	vcom := flag.Float64("vcom", 0, "VCOM in volts, as printed on the flex cable (required)")
	dev := flag.String("dev", "/dev/spidev0.0", "spidev device")
	card := flag.Bool("test-card", false, "show a test card instead of a Slide")
	flag.Parse()
	wantArgs := 1
	if *card {
		wantArgs = 0
	}
	if *vcom == 0 || flag.NArg() != wantArgs {
		fmt.Fprintln(os.Stderr, "usage: panel -vcom <volts> <slide.png> | -test-card")
		os.Exit(2)
	}
	if err := run(*dev, *vcom, *card, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "panel:", err)
		os.Exit(1)
	}
}

func run(dev string, vcom float64, card bool, path string) error {
	var s *image.Gray
	if card {
		s = testCard()
	} else {
		var err error
		if s, err = loadSlide(path); err != nil {
			return err
		}
	}
	bus, err := it8951.OpenSPIDev(dev)
	if err != nil {
		return err
	}
	defer bus.Close()
	p, err := it8951.Open(bus, vcom)
	if err != nil {
		return err
	}
	fmt.Printf("panel %dx%d\n", p.Width, p.Height)
	return p.Show(s)
}

func loadSlide(path string) (*image.Gray, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		return nil, fmt.Errorf("%s: not a grayscale Slide PNG", path)
	}
	return g, nil
}

// testCard has 16 gray bars, black to white left to right, over a strip of
// one-pixel lines and a 1:4 diagonal that show pixel order mistakes.
func testCard() *image.Gray {
	g := image.NewGray(image.Rect(0, 0, slide.Width, slide.Height))
	split := slide.Height * 2 / 3
	for y := range slide.Height {
		for x := range slide.Width {
			var v uint8
			switch {
			case y < split:
				v = uint8(x*16/slide.Width) * 17
			case x < slide.Width/2:
				v = 0xFF
				if x%8 == 0 || (y-split) == x/4 {
					v = 0
				}
			default:
				v = 0xFF * uint8(x%2)
			}
			g.Pix[y*g.Stride+x] = v
		}
	}
	return g
}
