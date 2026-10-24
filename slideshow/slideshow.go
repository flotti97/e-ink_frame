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
	"slices"
	"sync"
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

	refresh sync.Mutex // one Refresh at a time; guards Shuffle

	mu          sync.Mutex // guards the fields below; never held during a Refresh
	onPanel     string     // ID of the Slide the panel shows
	manual      uint64     // counts ShowNow Refreshes
	restartWait func()     // cuts the running wait short
}

// Run shows the next Slide every Interval until ctx is done. Problems with a
// single Slide are logged and the Slideshow goes on. A ShowNow restarts the
// Interval.
//
// The panel keeps its picture without power, so after a restart the Slide
// that was current stays up for a full Interval. Only when there is none, or
// it has been deleted, is a Slide shown straight away.
func (s *Slideshow) Run(ctx context.Context) error {
	s.refresh.Lock()
	if s.currentStillExists() {
		s.setOnPanel(s.Shuffle.Current)
	}
	s.refresh.Unlock()

	if s.Current() == "" {
		if err := s.Step(); err != nil {
			s.Log.Print(err)
		}
	}
	for {
		gen, err := s.wait(ctx)
		if err != nil {
			return err
		}
		if err := s.timedStep(gen); err != nil {
			s.Log.Print(err)
		}
	}
}

// wait blocks for one Interval, starting over whenever ShowNow puts a Slide
// up in the meantime. It returns the ShowNow generation its last wait began
// in.
func (s *Slideshow) wait(ctx context.Context) (uint64, error) {
	wait := s.Wait
	if wait == nil {
		wait = sleep
	}
	for {
		wctx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.restartWait = cancel
		gen := s.manual
		s.mu.Unlock()

		err := wait(wctx, s.Interval)
		cancel()
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if err == nil {
			return gen, nil
		}
		// Cut short by ShowNow: wait a full Interval from now.
	}
}

func (s *Slideshow) generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manual
}

// timedStep is the Step at the end of a wait that began in generation gen. A
// ShowNow that got to the panel after the wait ended, but before this Step,
// wins: its Slide stays up for a full Interval instead of being replaced.
func (s *Slideshow) timedStep(gen uint64) error {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if s.generation() != gen {
		return nil
	}
	return s.step()
}

func (s *Slideshow) currentStillExists() bool {
	if s.Shuffle.Current == "" {
		return false
	}
	_, err := os.Stat(s.Library.SlidePath(s.Shuffle.Current))
	return err == nil
}

// Current returns the ID of the Slide on the panel, or "" when none is known.
// After a restart that is the Slide saved as current, which the panel kept.
// It never waits for a Refresh in progress.
func (s *Slideshow) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.onPanel
}

func (s *Slideshow) setOnPanel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPanel = id
}

// Step picks the next Slide and shows it. An empty library shows nothing.
func (s *Slideshow) Step() error {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	return s.step()
}

func (s *Slideshow) step() error {
	ids, err := s.ids()
	if err != nil {
		return err
	}
	id, ok := s.Shuffle.Next(ids, s.Rand)
	if !ok {
		return nil
	}
	return s.show(id)
}

// ShowNow puts the Slide with this ID on the panel straight away. It counts
// towards the current round, and the next automatic Slide follows a full
// Interval later.
func (s *Slideshow) ShowNow(id string) error {
	s.refresh.Lock()
	defer s.refresh.Unlock()

	ids, err := s.ids()
	if err != nil {
		return err
	}
	if !slices.Contains(ids, id) {
		return fmt.Errorf("%s: %w", id, library.ErrNotFound)
	}
	s.Shuffle.Mark(id, ids)
	if err := s.show(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.manual++
	if s.restartWait != nil {
		s.restartWait()
	}
	return nil
}

func (s *Slideshow) ids() ([]string, error) {
	entries, err := s.Library.List()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids, nil
}

// show saves the Shuffle and does the Refresh. The Shuffle is saved first, so
// a Slide that can't be shown still counts and the Slideshow moves past it.
func (s *Slideshow) show(id string) error {
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
	s.setOnPanel(id)
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
