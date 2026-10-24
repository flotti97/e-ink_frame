// Command preview renders an Image as a Slide PNG, so a Slide can be checked
// on a PC without the panel.
//
//	preview <image.jpg|image.png> <slide.png>
package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/flotti97/e-ink_frame/slide"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: preview <image.jpg|image.png> <slide.png>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "preview:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := slide.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}

	w, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(w, slide.Render(img)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
