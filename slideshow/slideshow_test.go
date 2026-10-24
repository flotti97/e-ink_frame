package slideshow

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/flotti97/e-ink_frame/library"
	"github.com/flotti97/e-ink_frame/slide"
)

func TestFakePanelWritesTheSlideAsPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.png")
	s := image.NewGray(image.Rect(0, 0, slide.Width, slide.Height))
	s.Pix[0] = 255
	if err := (FakePanel{Path: path}).Show(s); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	g, ok := got.(*image.Gray)
	if !ok || g.Bounds() != s.Bounds() || g.Pix[0] != 255 {
		t.Errorf("panel PNG does not hold the Slide")
	}
}

// recordingPanel remembers which Slide it was shown by its first pixel.
type recordingPanel struct{ shown []uint8 }

func (p *recordingPanel) Show(s *image.Gray) error {
	p.shown = append(p.shown, s.Pix[0])
	return nil
}

// fakeLibrary serves Slides whose first pixel identifies them.
type fakeLibrary struct{ dir string }

func newFakeLibrary(t *testing.T, n int) fakeLibrary {
	l := fakeLibrary{dir: t.TempDir()}
	for i := 0; i < n; i++ {
		s := image.NewGray(image.Rect(0, 0, 4, 3))
		s.Pix[0] = uint8(i)
		var b bytes.Buffer
		png.Encode(&b, s)
		os.WriteFile(l.SlidePath(string(rune('a'+i))), b.Bytes(), 0o644)
	}
	return l
}

func (l fakeLibrary) List() ([]library.Entry, error) {
	files, _ := filepath.Glob(filepath.Join(l.dir, "*.png"))
	var out []library.Entry
	for _, f := range files {
		out = append(out, library.Entry{ID: filepath.Base(f[:len(f)-4])})
	}
	return out, nil
}

func (l fakeLibrary) SlidePath(id string) string { return filepath.Join(l.dir, id+".png") }

func TestRunShowsNextSlideEveryInterval(t *testing.T) {
	panel := &recordingPanel{}
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	show := &Slideshow{
		Library:   newFakeLibrary(t, 4),
		Panel:     panel,
		Shuffle:   &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Interval:  7 * time.Minute,
		Rand:      newRand(),
		Log:       log.New(io.Discard, "", 0),
		Wait: func(ctx context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) == 8 {
				cancel()
			}
			return ctx.Err()
		},
	}

	if err := show.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
	if len(panel.shown) != 8 {
		t.Fatalf("showed %d Slides over 8 intervals, want 8", len(panel.shown))
	}
	for _, d := range waits {
		if d != 7*time.Minute {
			t.Errorf("waited %v between Slides, want 7m", d)
		}
	}
	for _, round := range [][]uint8{panel.shown[:4], panel.shown[4:]} {
		sorted := slices.Sorted(slices.Values(round))
		if !slices.Equal(sorted, []uint8{0, 1, 2, 3}) {
			t.Errorf("round %v is not each Slide once", round)
		}
	}
}

func TestRunSavesShuffleAfterEverySlide(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	ctx, cancel := context.WithCancel(context.Background())
	show := &Slideshow{
		Library: newFakeLibrary(t, 5), Panel: &recordingPanel{}, Shuffle: &Shuffle{},
		StatePath: state, Interval: time.Minute, Rand: newRand(),
		Log:  log.New(io.Discard, "", 0),
		Wait: func(context.Context, time.Duration) error { cancel(); return ctx.Err() },
	}
	show.Run(ctx)

	saved, err := LoadShuffle(state)
	if err != nil || saved.Current != show.Shuffle.Current || len(saved.Shown) != 1 {
		t.Errorf("saved %+v (%v), want the state after one Slide", saved, err)
	}
}

func TestRunKeepsGoingWhenTheLibraryIsEmptyOrASlideIsMissing(t *testing.T) {
	lib := newFakeLibrary(t, 0)
	panel := &recordingPanel{}
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	show := &Slideshow{
		Library: lib, Panel: panel, Shuffle: &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "state.json"), Interval: time.Minute, Rand: newRand(),
		Log: log.New(io.Discard, "", 0),
		Wait: func(context.Context, time.Duration) error {
			n++
			switch n {
			case 1: // a broken Slide appears in the empty library
				os.WriteFile(lib.SlidePath("bad"), []byte("not a png"), 0o644)
			case 2:
				os.Remove(lib.SlidePath("bad"))
				s := image.NewGray(image.Rect(0, 0, 4, 3))
				s.Pix[0] = 9
				var b bytes.Buffer
				png.Encode(&b, s)
				os.WriteFile(lib.SlidePath("good"), b.Bytes(), 0o644)
			case 3:
				cancel()
			}
			return ctx.Err()
		},
	}
	show.Run(ctx)
	if !slices.Equal(panel.shown, []uint8{9}) {
		t.Errorf("panel showed %v, want only the good Slide", panel.shown)
	}
}

func TestRestartLeavesTheCurrentSlideUpForAnInterval(t *testing.T) {
	lib := newFakeLibrary(t, 3)
	panel := &recordingPanel{}
	ctx, cancel := context.WithCancel(context.Background())
	show := &Slideshow{
		Library: lib, Panel: panel, Shuffle: &Shuffle{Shown: []string{"b"}, Current: "b"},
		StatePath: filepath.Join(t.TempDir(), "state.json"), Interval: time.Minute, Rand: newRand(),
		Log:  log.New(io.Discard, "", 0),
		Wait: func(context.Context, time.Duration) error { cancel(); return ctx.Err() },
	}
	show.Run(ctx)
	if len(panel.shown) != 0 {
		t.Errorf("restart replaced the Slide on the panel straight away: showed %v", panel.shown)
	}
}

func TestRestartShowsASlideAtOnceWhenTheCurrentOneIsGone(t *testing.T) {
	panel := &recordingPanel{}
	ctx, cancel := context.WithCancel(context.Background())
	show := &Slideshow{
		Library: newFakeLibrary(t, 3), Panel: panel, Shuffle: &Shuffle{Current: "deleted"},
		StatePath: filepath.Join(t.TempDir(), "state.json"), Interval: time.Minute, Rand: newRand(),
		Log:  log.New(io.Discard, "", 0),
		Wait: func(context.Context, time.Duration) error { cancel(); return ctx.Err() },
	}
	show.Run(ctx)
	if len(panel.shown) != 1 {
		t.Errorf("showed %d Slides before the first wait, want 1", len(panel.shown))
	}
}
