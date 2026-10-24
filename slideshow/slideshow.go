// Package slideshow runs the Slideshow: every interval it picks the next
// Slide from the Image library and shows it on the panel.
package slideshow

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/flotti97/e-ink_frame/internal/atomicfile"
	"github.com/flotti97/e-ink_frame/library"
)

// Panel shows a Slide with one full Refresh.
type Panel interface {
	Show(s *image.Gray) error
}

// FakePanel stands in for the e-paper panel on a PC by writing the Slide it
// is shown to a PNG file.
type FakePanel struct {
	Path string
}

// Show writes s to p.Path.
func (p FakePanel) Show(s *image.Gray) error {
	var b bytes.Buffer
	if err := png.Encode(&b, s); err != nil {
		return err
	}
	return atomicfile.Write(p.Path, b.Bytes())
}

// Library is the part of library.Library the Slideshow uses.
type Library interface {
	List() ([]library.Entry, error)
	SlidePath(id string) string
}

// Slideshow shows the next Slide every Interval.
type Slideshow struct {
	Library   Library
	Panel     Panel
	Shuffle   *Shuffle
	StatePath string // where Shuffle is saved after every pick
	Interval  time.Duration
	Rand      *rand.Rand
	Log       *log.Logger

	// Wait blocks for d or until ctx is done. Nil means a real timer.
	Wait func(ctx context.Context, d time.Duration) error
}

// Run shows the next Slide every Interval until ctx is done. Problems with a
// single Slide are logged and the Slideshow goes on.
//
// The panel keeps its picture without power, so after a restart the Slide
// that was current stays up for a full Interval. Only when there is none, or
// it has been deleted, is a Slide shown straight away.
func (s *Slideshow) Run(ctx context.Context) error {
	wait := s.Wait
	if wait == nil {
		wait = sleep
	}
	if s.currentStillExists() {
		if err := wait(ctx, s.Interval); err != nil {
			return err
		}
	}
	for {
		if err := s.Step(); err != nil {
			s.Log.Print(err)
		}
		if err := wait(ctx, s.Interval); err != nil {
			return err
		}
	}
}

func (s *Slideshow) currentStillExists() bool {
	if s.Shuffle.Current == "" {
		return false
	}
	_, err := os.Stat(s.Library.SlidePath(s.Shuffle.Current))
	return err == nil
}

// Step picks the next Slide and shows it. An empty library shows nothing.
func (s *Slideshow) Step() error {
	entries, err := s.Library.List()
	if err != nil {
		return err
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	id, ok := s.Shuffle.Next(ids, s.Rand)
	if !ok {
		return nil
	}
	// Save before showing, so a Slide that can't be shown still counts and
	// the Slideshow moves past it.
	if err := s.Shuffle.Save(s.StatePath); err != nil {
		s.Log.Printf("saving shuffle: %v", err)
	}

	sl, err := loadSlide(s.Library.SlidePath(id))
	if err != nil {
		return fmt.Errorf("Slide %s: %w", id, err)
	}
	if err := s.Panel.Show(sl); err != nil {
		return fmt.Errorf("showing Slide %s: %w", id, err)
	}
	return nil
}

func loadSlide(path string) (*image.Gray, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if g, ok := img.(*image.Gray); ok {
		return g, nil
	}
	g := image.NewGray(img.Bounds())
	draw.Draw(g, g.Bounds(), img, img.Bounds().Min, draw.Src)
	return g, nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
