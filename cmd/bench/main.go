// Command bench times partial Refreshes on the real panel, to see how fast
// small areas can animate in each Mode. It runs on the Pi.
//
//	bench -vcom -1.58
//
// For each Mode it blinks a patch, moves a square across the screen and
// refreshes the whole panel, prints the timings, then leaves the result up
// for a moment so ghosting can be judged before a full Refresh cleans up.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"os"
	"time"

	"github.com/flotti97/e-ink_frame/it8951"
	"github.com/flotti97/e-ink_frame/slide"
)

var modes = []struct {
	name string
	m    it8951.Mode
}{
	{"GC16", it8951.ModeGC16},
	{"GL16", it8951.ModeGL16},
	{"DU", it8951.ModeDU},
	{"A2", it8951.ModeA2},
}

func main() {
	vcom := flag.Float64("vcom", 0, "VCOM in volts, as printed on the flex cable (required)")
	dev := flag.String("dev", "/dev/spidev0.0", "spidev device")
	frames := flag.Int("frames", 10, "Refreshes per test")
	look := flag.Duration("look", 5*time.Second, "time to look at each Mode's ghosting before cleaning up")
	flag.Parse()
	if *vcom == 0 || flag.NArg() != 0 || *frames < 1 {
		fmt.Fprintln(os.Stderr, "usage: bench -vcom <volts> [-frames n] [-look 5s]")
		os.Exit(2)
	}
	if err := run(*dev, *vcom, *frames, *look); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func run(dev string, vcom float64, frames int, look time.Duration) error {
	bus, err := it8951.OpenSPIDev(dev)
	if err != nil {
		return err
	}
	defer bus.Close()
	p, err := it8951.Open(bus, vcom)
	if err != nil {
		return err
	}
	defer p.Sleep()
	fmt.Printf("panel %dx%d, %d frames per test\n\n", p.Width, p.Height, frames)
	fmt.Printf("%-5s %-14s %9s %9s %9s %6s\n", "mode", "test", "area", "load", "refresh", "fps")

	bg := background()
	full := bg.Bounds()
	canvas := image.NewGray(full)
	for _, mode := range modes {
		clean := func() error {
			copy(canvas.Pix, bg.Pix)
			if err := p.Load(canvas, full); err != nil {
				return err
			}
			return p.Display(full, it8951.ModeGC16)
		}
		if err := clean(); err != nil {
			return err
		}
		tests := []struct {
			name string
			step func(i int) image.Rectangle
		}{
			{"blink 256x128", blink(canvas, bg)},
			{"move 64x64", move(canvas, bg, frames)},
			{"full screen", func(i int) image.Rectangle {
				invert(canvas, full)
				return full
			}},
		}
		for _, t := range tests {
			var load, refresh time.Duration
			var area image.Rectangle
			for i := range frames {
				r := t.step(i)
				area = r
				start := time.Now()
				if err := p.Load(canvas, r); err != nil {
					return err
				}
				loaded := time.Now()
				if err := p.Display(r, mode.m); err != nil {
					return err
				}
				load += loaded.Sub(start)
				refresh += time.Since(loaded)
			}
			n := time.Duration(frames)
			fmt.Printf("%-5s %-14s %9s %7dms %7dms %6.1f\n", mode.name, t.name,
				fmt.Sprintf("%dx%d", area.Dx(), area.Dy()),
				(load / n).Milliseconds(), (refresh / n).Milliseconds(),
				float64(frames)/(load+refresh).Seconds())
			// Leave the last full-screen frame up with the test's ghosts.
			if t.name != "full screen" {
				continue
			}
			invert(canvas, full)
			if err := p.Load(canvas, full); err != nil {
				return err
			}
			if err := p.Display(full, mode.m); err != nil {
				return err
			}
		}
		time.Sleep(look)
	}
	return nil
}

// blink toggles a patch in the upper gray bars between black and the
// background, like an eye closing and opening.
func blink(canvas, bg *image.Gray) func(int) image.Rectangle {
	r := image.Rect(slide.Width/2-128, slide.Height/4, slide.Width/2+128, slide.Height/4+128)
	return func(i int) image.Rectangle {
		if i%2 == 0 {
			draw.Draw(canvas, r, image.Black, image.Point{}, draw.Src)
		} else {
			draw.Draw(canvas, r, bg, r.Min, draw.Src)
		}
		return r
	}
}

// move slides a black square across the gray bars, refreshing the area that
// covers both its old and new position, like a bird flying across a photo.
func move(canvas, bg *image.Gray, frames int) func(int) image.Rectangle {
	const size = 64
	y := slide.Height / 2
	stride := (slide.Width - size) / frames &^ 3
	var prev image.Rectangle
	return func(i int) image.Rectangle {
		cur := image.Rect(i*stride, y, i*stride+size, y+size)
		draw.Draw(canvas, prev, bg, prev.Min, draw.Src)
		draw.Draw(canvas, cur, image.Black, image.Point{}, draw.Src)
		r := cur
		if !prev.Empty() {
			r = r.Union(prev)
		}
		prev = cur
		return r
	}
}

func invert(g *image.Gray, r image.Rectangle) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := g.Pix[g.PixOffset(r.Min.X, y):g.PixOffset(r.Max.X, y)]
		for x := range row {
			row[x] = ^row[x]
		}
	}
}

// background has 16 gray bars, black to white left to right, so damage to
// gray levels around the animated areas shows.
func background() *image.Gray {
	g := image.NewGray(image.Rect(0, 0, slide.Width, slide.Height))
	for y := range slide.Height {
		for x := range slide.Width {
			g.Pix[y*g.Stride+x] = uint8(x*16/slide.Width) * 17
		}
	}
	return g
}
