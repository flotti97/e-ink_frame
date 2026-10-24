package slideshow

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"

	"github.com/flotti97/e-ink_frame/internal/atomicfile"
)

// Shuffle picks Slides in random order so that every Slide is shown once
// before any repeats. Its state is small enough to save after every pick, so
// a restart carries on with the same round.
type Shuffle struct {
	Shown   []string `json:"shown"`   // IDs shown in the current round
	Current string   `json:"current"` // ID on the panel now
}

// Next picks the next Slide from ids, the Slides that exist right now. Slides
// added mid-round are still to come this round; deleted ones are dropped.
// It reports false only when ids is empty.
func (s *Shuffle) Next(ids []string, r *rand.Rand) (string, bool) {
	if len(ids) == 0 {
		return "", false
	}
	s.Shown = slices.DeleteFunc(s.Shown, func(id string) bool { return !slices.Contains(ids, id) })

	left := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return slices.Contains(s.Shown, id) })
	if len(left) == 0 {
		// New round. Hold back the Slide on the panel so it isn't shown twice
		// in a row, unless it is the only one.
		s.Shown = nil
		left = slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == s.Current })
		if len(left) == 0 {
			left = ids
		}
	}

	pick := left[r.IntN(len(left))]
	s.Shown = append(s.Shown, pick)
	s.Current = pick
	return pick, true
}

// ErrCorruptState means the saved Shuffle could not be parsed.
var ErrCorruptState = errors.New("corrupt shuffle state")

// LoadShuffle reads a saved Shuffle. A missing file gives a fresh Shuffle. A
// corrupt one also gives a fresh Shuffle, together with ErrCorruptState so the
// caller can report it and carry on. Any other read error returns no Shuffle,
// so a file that is only temporarily unreadable is never overwritten.
func LoadShuffle(path string) (*Shuffle, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Shuffle{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Shuffle
	if err := json.Unmarshal(data, &s); err != nil {
		return &Shuffle{}, fmt.Errorf("%s: %w: %v", path, ErrCorruptState, err)
	}
	return &s, nil
}

// Save writes the Shuffle to path.
func (s *Shuffle) Save(path string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, data)
}
