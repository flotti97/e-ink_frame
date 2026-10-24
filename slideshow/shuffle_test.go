package slideshow

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('a' + i))
	}
	return out
}

func newRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func TestEverySlideOnceBeforeAnyRepeats(t *testing.T) {
	all := ids(7)
	var s Shuffle
	r := newRand()
	for round := 0; round < 5; round++ {
		var seen []string
		for range all {
			id, ok := s.Next(all, r)
			if !ok {
				t.Fatal("no Slide picked")
			}
			if slices.Contains(seen, id) {
				t.Fatalf("round %d: %s repeated before all were shown (%v)", round, id, seen)
			}
			seen = append(seen, id)
		}
	}
}

func TestOrderIsShuffled(t *testing.T) {
	all := ids(10)
	var s Shuffle
	r := newRand()
	var first, second []string
	for range all {
		id, _ := s.Next(all, r)
		first = append(first, id)
	}
	for range all {
		id, _ := s.Next(all, r)
		second = append(second, id)
	}
	if slices.Equal(first, all) || slices.Equal(first, second) {
		t.Errorf("order does not look shuffled: %v then %v", first, second)
	}
}

func TestNoBackToBackRepeatAcrossRounds(t *testing.T) {
	all := ids(3)
	var s Shuffle
	r := newRand()
	prev := ""
	for i := 0; i < 300; i++ {
		id, _ := s.Next(all, r)
		if id == prev {
			t.Fatalf("step %d: %s shown twice in a row", i, id)
		}
		prev = id
	}
}

func TestSingleSlideKeepsShowing(t *testing.T) {
	var s Shuffle
	r := newRand()
	for i := 0; i < 3; i++ {
		if id, ok := s.Next([]string{"only"}, r); !ok || id != "only" {
			t.Fatalf("got %q, %v", id, ok)
		}
	}
}

func TestEmptyLibraryPicksNothing(t *testing.T) {
	var s Shuffle
	if _, ok := s.Next(nil, newRand()); ok {
		t.Error("picked a Slide from an empty library")
	}
}

func TestAddedSlidesJoinTheCurrentRoundAndDeletedOnesLeaveIt(t *testing.T) {
	var s Shuffle
	r := newRand()
	all := ids(4)
	a, _ := s.Next(all, r)
	b, _ := s.Next(all, r)

	// b is deleted, z is added mid-round.
	now := slices.DeleteFunc(slices.Clone(all), func(id string) bool { return id == b })
	now = append(now, "z")

	var rest []string
	for i := 0; i < len(now)-1; i++ {
		id, _ := s.Next(now, r)
		rest = append(rest, id)
	}
	if slices.Contains(rest, a) || slices.Contains(rest, b) {
		t.Errorf("rest of the round %v repeats %s or shows deleted %s", rest, a, b)
	}
	if !slices.Contains(rest, "z") {
		t.Errorf("added Slide z missing from the rest of the round %v", rest)
	}
}

func TestShuffleSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	all := ids(6)
	r := newRand()

	s, err := LoadShuffle(path)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for i := 0; i < 3; i++ {
		id, _ := s.Next(all, r)
		seen = append(seen, id)
		if err := s.Save(path); err != nil {
			t.Fatal(err)
		}
	}

	restarted, err := LoadShuffle(path)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Current != seen[2] {
		t.Errorf("current after restart is %q, want %q", restarted.Current, seen[2])
	}
	for i := 0; i < 3; i++ {
		id, _ := restarted.Next(all, r)
		if slices.Contains(seen, id) {
			t.Fatalf("after restart %s repeated before the round finished (%v)", id, seen)
		}
		seen = append(seen, id)
	}
}

func TestLoadShuffleWithoutStateStartsFresh(t *testing.T) {
	s, err := LoadShuffle(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(s.Shown) != 0 || s.Current != "" {
		t.Errorf("got %+v, %v", s, err)
	}
}

func TestLoadShuffleResetsOnlyCorruptState(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.json")
	os.WriteFile(corrupt, []byte("{nope"), 0o644)
	s, err := LoadShuffle(corrupt)
	if !errors.Is(err, ErrCorruptState) || s == nil {
		t.Errorf("corrupt state: got %v, %v; want a fresh Shuffle and ErrCorruptState", s, err)
	}

	// A file that can't be read at all (here: a directory) must not be
	// replaced by a fresh Shuffle and overwritten later.
	s, err = LoadShuffle(dir)
	if err == nil || errors.Is(err, ErrCorruptState) || s != nil {
		t.Errorf("unreadable state: got %v, %v; want a plain error and no Shuffle", s, err)
	}
}

func TestMarkCountsAManualPickInTheRound(t *testing.T) {
	all := ids(5)
	var s Shuffle
	r := newRand()
	first, _ := s.Next(all, r)

	// Pick one that hasn't been shown yet this round.
	manual := all[0]
	if manual == first {
		manual = all[1]
	}
	s.Mark(manual, all)
	if s.Current != manual {
		t.Errorf("current is %q, want %q", s.Current, manual)
	}

	seen := []string{first, manual}
	for i := 0; i < len(all)-2; i++ {
		id, _ := s.Next(all, r)
		if slices.Contains(seen, id) {
			t.Fatalf("%s repeated before the round finished (%v)", id, seen)
		}
		seen = append(seen, id)
	}
}

func TestMarkingAnAlreadyShownSlideDoesNotRestartTheRound(t *testing.T) {
	all := ids(4)
	var s Shuffle
	r := newRand()
	a, _ := s.Next(all, r)
	b, _ := s.Next(all, r)

	s.Mark(a, all)

	var rest []string
	for i := 0; i < 2; i++ {
		id, _ := s.Next(all, r)
		rest = append(rest, id)
	}
	if slices.Contains(rest, a) || slices.Contains(rest, b) {
		t.Errorf("rest of the round %v repeats %s or %s", rest, a, b)
	}
}

func TestMarkingInAFinishedRoundStartsTheNextOne(t *testing.T) {
	all := ids(3)
	var s Shuffle
	r := newRand()
	for range all {
		s.Next(all, r)
	}
	s.Mark(all[0], all)
	if !slices.Equal(s.Shown, []string{all[0]}) {
		t.Errorf("shown %v, want a new round holding only %s", s.Shown, all[0])
	}
}
