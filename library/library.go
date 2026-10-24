// Package library stores Images on disk together with the Slide derived from
// each one.
//
// Layout under the library directory:
//
//	images/<id>.<jpg|png>  the Image exactly as supplied
//	images/<id>.json       its Entry; written last, so it marks a complete add
//	slides/<id>.png        its Slide
package library

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/flotti97/e-ink_frame/slide"
)

// ErrNotFound is returned when no Image has the given ID.
var ErrNotFound = errors.New("no such Image")

// Entry describes one stored Image.
type Entry struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"` // file name it was added under
	Ext   string    `json:"ext"`  // "jpg" or "png"
	Added time.Time `json:"added"`
}

// Library is a directory of Images and their Slides.
type Library struct {
	dir string
}

// Open uses dir as a Library, creating it if needed.
func Open(dir string) (*Library, error) {
	for _, sub := range []string{"images", "slides"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Library{dir: dir}, nil
}

// ImagePath is where the Image of e is kept, unchanged.
func (l *Library) ImagePath(e Entry) string {
	return filepath.Join(l.dir, "images", e.ID+"."+e.Ext)
}

// SlidePath is where the Slide of the Image with this ID is kept.
func (l *Library) SlidePath(id string) string {
	return filepath.Join(l.dir, "slides", id+".png")
}

func (l *Library) entryPath(id string) string {
	return filepath.Join(l.dir, "images", id+".json")
}

// Add stores a JPEG or PNG Image under name and derives its Slide. Adding the
// same bytes again returns the existing Entry. Other formats and unreadable
// files are rejected with an error naming the file, and nothing is stored.
func (l *Library) Add(name string, r io.Reader) (Entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	img, err := slide.Decode(bytes.NewReader(data))
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}

	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:8])
	if e, err := l.get(id); err == nil {
		return e, nil
	}

	e := Entry{ID: id, Name: name, Ext: "png", Added: time.Now().UTC()}
	if http.DetectContentType(data) == "image/jpeg" {
		e.Ext = "jpg"
	}

	var s bytes.Buffer
	if err := png.Encode(&s, slide.Render(img)); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	meta, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}

	if err := writeFile(l.SlidePath(id), s.Bytes()); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := writeFile(l.ImagePath(e), data); err != nil {
		os.Remove(l.SlidePath(id))
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := writeFile(l.entryPath(id), meta); err != nil {
		os.Remove(l.SlidePath(id))
		os.Remove(l.ImagePath(e))
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	return e, nil
}

// List returns every stored Image, oldest first.
func (l *Library) List() ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(l.dir, "images", "*.json"))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		e, err := l.get(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return a.Added.Compare(b.Added) })
	return entries, nil
}

// Delete removes the Image with this ID and its Slide.
func (l *Library) Delete(id string) error {
	e, err := l.get(id)
	if err != nil {
		return err
	}
	// The Entry goes first so a half-finished delete never lists a missing Image.
	for _, p := range []string{l.entryPath(id), l.SlidePath(id), l.ImagePath(e)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (l *Library) get(id string) (Entry, error) {
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return Entry{}, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	data, err := os.ReadFile(l.entryPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, fmt.Errorf("%s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, fmt.Errorf("%s: corrupt entry: %w", id, err)
	}
	return e, nil
}

// writeFile writes via a temporary file and rename, so readers never see a
// partly written file.
func writeFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync() // the Frame may lose power at any time
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
