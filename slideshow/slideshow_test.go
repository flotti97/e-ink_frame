package slideshow

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/flotti97/e-ink_frame/clock"
	"github.com/flotti97/e-ink_frame/it8951"
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

// recordingPanel remembers which Slide it was shown by its first pixel,
// and the areas of partial Refreshes.
type recordingPanel struct {
	shown   []uint8
	updates []image.Rectangle
}

func (p *recordingPanel) Show(s *image.Gray) error {
	p.shown = append(p.shown, s.Pix[0])
	return nil
}

func (p *recordingPanel) Update(_ *image.Gray, r image.Rectangle, _ it8951.Mode) error {
	p.updates = append(p.updates, r)
	return nil
}

func (p *recordingPanel) Sleep() error { return nil }

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

func newShow(t *testing.T, n int, wait func(context.Context, time.Duration) error) (*Slideshow, *recordingPanel) {
	panel := &recordingPanel{}
	return &Slideshow{
		Library: newFakeLibrary(t, n), Panel: panel, Shuffle: &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "state.json"), Interval: time.Minute, Rand: newRand(),
		Log: log.New(io.Discard, "", 0), Wait: wait,
	}, panel
}

func TestShowNowPutsTheSlideUpAtOnce(t *testing.T) {
	show, panel := newShow(t, 4, nil)
	if err := show.ShowNow("c"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(panel.shown, []uint8{2}) {
		t.Errorf("panel showed %v, want Slide c", panel.shown)
	}
	if show.Current() != "c" {
		t.Errorf("Current() = %q, want c", show.Current())
	}
	saved, _ := LoadShuffle(show.StatePath)
	if saved.Current != "c" || !slices.Contains(saved.Shown, "c") {
		t.Errorf("saved shuffle %+v does not record c", saved)
	}
}

func TestShowNowRestartsTheIntervalAndKeepsTheRound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var show *Slideshow
	var waits int
	show, panel := newShow(t, 4, func(wctx context.Context, d time.Duration) error {
		waits++
		switch waits {
		case 1: // a "show now" press while waiting
			manual := "a"
			if show.Current() == "a" {
				manual = "b"
			}
			if err := show.ShowNow(manual); err != nil {
				t.Error(err)
			}
			<-wctx.Done() // the press cuts this wait short
			return wctx.Err()
		case 2, 3:
			return nil
		default:
			cancel()
			return ctx.Err()
		}
	})
	show.Run(ctx)

	// 1 automatic, 1 manual, then the 2 left in the round: each Slide once.
	if len(panel.shown) != 4 {
		t.Fatalf("panel showed %v, want 4 Slides", panel.shown)
	}
	if got := slices.Sorted(slices.Values(panel.shown)); !slices.Equal(got, []uint8{0, 1, 2, 3}) {
		t.Errorf("round %v repeats a Slide", panel.shown)
	}
}

func TestShowNowUnknownSlide(t *testing.T) {
	show, panel := newShow(t, 2, nil)
	if err := show.ShowNow("zzz"); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("err = %v, want library.ErrNotFound", err)
	}
	if len(panel.shown) != 0 {
		t.Error("panel changed")
	}
}

func TestCurrentIsWhatIsReallyOnThePanel(t *testing.T) {
	show, _ := newShow(t, 0, nil)
	lib := show.Library.(fakeLibrary)
	os.WriteFile(lib.SlidePath("broken"), []byte("not a png"), 0o644)

	if err := show.Step(); err == nil {
		t.Fatal("broken Slide showed without error")
	}
	if show.Current() != "" {
		t.Errorf("Current() = %q after a failed Refresh, want nothing", show.Current())
	}
}

func TestCurrentAfterRestartIsTheSavedSlide(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	show, _ := newShow(t, 3, func(context.Context, time.Duration) error { cancel(); return ctx.Err() })
	show.Shuffle = &Shuffle{Shown: []string{"b"}, Current: "b"}
	show.Run(ctx)
	if show.Current() != "b" {
		t.Errorf("Current() = %q, want the Slide left on the panel, b", show.Current())
	}
}

// blockingPanel holds every Refresh until release is closed.
type blockingPanel struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingPanel) Show(*image.Gray) error {
	p.started <- struct{}{}
	<-p.release
	return nil
}

func (p *blockingPanel) Update(*image.Gray, image.Rectangle, it8951.Mode) error { return nil }
func (p *blockingPanel) Sleep() error                                           { return nil }

func TestCurrentAnswersDuringASlowRefresh(t *testing.T) {
	show, _ := newShow(t, 3, nil)
	panel := &blockingPanel{started: make(chan struct{}), release: make(chan struct{})}
	show.Panel = panel
	defer close(panel.release)

	go show.Step()
	<-panel.started

	done := make(chan struct{})
	go func() { show.Current(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Current() blocked while the panel was refreshing")
	}
}

func TestATimedStepDoesNotOverrideAShowNowThatBeatItToThePanel(t *testing.T) {
	show, panel := newShow(t, 3, nil)
	gen := show.generation()                  // the timer's wait began here...
	if err := show.ShowNow("b"); err != nil { // ...a press landed as it fired...
		t.Fatal(err)
	}
	if err := show.timedStep(gen); err != nil { // ...and then the timed Step ran.
		t.Fatal(err)
	}
	if !slices.Equal(panel.shown, []uint8{1}) || show.Current() != "b" {
		t.Errorf("panel showed %v, current %q; want b to stay up", panel.shown, show.Current())
	}
}

// framePanel keeps a copy of the whole panel, as Show and Update leave it,
// and counts the quick Updates and whether it was left asleep.
type framePanel struct {
	img    *image.Gray
	quick  int
	asleep bool
}

func (p *framePanel) Show(s *image.Gray) error {
	p.img = image.NewGray(s.Bounds())
	copy(p.img.Pix, s.Pix)
	p.asleep = true
	return nil
}

func (p *framePanel) Update(s *image.Gray, r image.Rectangle, m it8951.Mode) error {
	draw.Draw(p.img, r, s, r.Min, draw.Src)
	if m != it8951.ModeGL16 {
		p.quick++
	}
	p.asleep = false
	return nil
}

func (p *framePanel) Sleep() error { p.asleep = true; return nil }

func TestClockIsWrittenOntoTheSlideAndRewrittenOnTick(t *testing.T) {
	lib := fakeLibrary{dir: t.TempDir()}
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	for i := range s.Pix {
		s.Pix[i] = 0xC0
	}
	var b bytes.Buffer
	png.Encode(&b, s)
	os.WriteFile(lib.SlidePath("a"), b.Bytes(), 0o644)

	now := time.Date(2026, 10, 24, 14, 20, 0, 0, time.UTC)
	panel := &framePanel{}
	show := &Slideshow{
		Library: lib, Panel: panel, Shuffle: &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "shuffle.json"),
		Rand:      newRand(), Log: log.New(io.Discard, "", 0),
		Clock: inkClock, Now: func() time.Time { return now },
	}
	if err := show.Tick(); err != nil {
		t.Fatal(err)
	}
	if panel.img != nil {
		t.Fatal("Tick before any Slide touched the panel")
	}
	if err := show.Step(); err != nil {
		t.Fatal(err)
	}
	// written is what the Slide with the time in handwriting looks like.
	written := func() *image.Gray {
		w := image.NewGray(s.Bounds())
		copy(w.Pix, s.Pix)
		for _, st := range clock.Steps(now, w.Bounds(), 20) {
			st.Draw(w, clock.Ink(s, clock.Area(s.Bounds())))
		}
		return w
	}
	if !bytes.Equal(panel.img.Pix, written().Pix) {
		t.Fatal("panel does not show the Slide with the time written on it")
	}
	if steps := len(clock.Steps(now, s.Bounds(), 20)); panel.quick != steps || !panel.asleep {
		t.Errorf("%d quick Updates for %d steps, asleep %v", panel.quick, steps, panel.asleep)
	}

	now = now.Add(5 * time.Minute)
	if err := show.Tick(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(panel.img.Pix, written().Pix) {
		t.Error("Tick did not replace the time")
	}
}

func TestSetClockRubsOutAndRewritesTheTime(t *testing.T) {
	lib := fakeLibrary{dir: t.TempDir()}
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	var b bytes.Buffer
	png.Encode(&b, s)
	os.WriteFile(lib.SlidePath("a"), b.Bytes(), 0o644)
	config := filepath.Join(t.TempDir(), "frame.json")
	os.WriteFile(config, []byte(`{"vcom": -1.58}`), 0o644)

	panel := &framePanel{}
	show := &Slideshow{
		Library: lib, Panel: panel, Shuffle: &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "shuffle.json"),
		Rand:      newRand(), Log: log.New(io.Discard, "", 0),
		Clock: inkClock, ConfigPath: config,
	}
	if err := show.Step(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(panel.img.Pix, s.Pix) {
		t.Fatal("no time written")
	}

	if err := show.SetClock(ClockSettings{Step: 20, Style: StyleInk}); err != nil {
		t.Fatal(err)
	}
	if show.ClockOn() {
		t.Error("Clock still on")
	}
	if err := show.Tick(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(panel.img.Pix, s.Pix) || !panel.asleep {
		t.Error("time not rubbed out")
	}
	if c, err := LoadConfig(config); err != nil || c.Clock.On || c.VCOM != -1.58 {
		t.Errorf("saved config = %+v, %v", c, err)
	}

	quick := panel.quick
	if err := show.Tick(); err != nil {
		t.Fatal(err)
	}
	if err := show.SetClock(inkClock); err != nil {
		t.Fatal(err)
	}
	if err := show.Tick(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(panel.img.Pix, s.Pix) || panel.quick == quick {
		t.Error("time not written again")
	}
}

func TestSetClockOnAfterARestartShowsTheSlideAgain(t *testing.T) {
	show, panel := newShow(t, 3, nil)
	show.Shuffle.Current = "b"
	show.setOnPanel("b")
	if err := show.SetClock(inkClock); err != nil {
		t.Fatal(err)
	}
	// The fake Slides are too small to write on; only the full Refresh counts.
	show.Tick()
	if len(panel.shown) != 1 || panel.shown[0] != 1 {
		t.Errorf("panel shown %v, want Slide 1 again", panel.shown)
	}
}

var inkClock = ClockSettings{On: true, Step: 20, Style: StyleInk}

func TestChalkWritesWhiteOnABlackBand(t *testing.T) {
	lib := fakeLibrary{dir: t.TempDir()}
	s := image.NewGray(image.Rect(0, 0, 1872, 1404))
	for i := range s.Pix {
		s.Pix[i] = 0xC0
	}
	var b bytes.Buffer
	png.Encode(&b, s)
	os.WriteFile(lib.SlidePath("a"), b.Bytes(), 0o644)
	panel := &framePanel{}
	show := &Slideshow{
		Library: lib, Panel: panel, Shuffle: &Shuffle{},
		StatePath: filepath.Join(t.TempDir(), "shuffle.json"),
		Rand:      newRand(), Log: log.New(io.Discard, "", 0),
		Clock: ClockSettings{On: true, Step: 20, Style: StyleChalk},
	}
	if err := show.Step(); err != nil {
		t.Fatal(err)
	}
	band := clock.Band(s.Bounds())
	var white int
	for y := range s.Bounds().Dy() {
		for x := range s.Bounds().Dx() {
			v := panel.img.GrayAt(x, y).Y
			switch {
			case !image.Pt(x, y).In(band) && v != 0xC0:
				t.Fatalf("pixel %d,%d outside the band changed", x, y)
			case image.Pt(x, y).In(band) && v != 0 && v != 0xFF:
				t.Fatalf("pixel %d,%d in the band is gray", x, y)
			case v == 0xFF:
				white++
			}
		}
	}
	if white == 0 {
		t.Error("nothing written in white")
	}
}

func TestSetClockRejectsBadSettings(t *testing.T) {
	show, _ := newShow(t, 1, nil)
	for _, c := range []ClockSettings{{Step: 0, Style: StyleInk}, {Step: 20, Style: "crayon"}} {
		if err := show.SetClock(c); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
}
