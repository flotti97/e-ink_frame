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
	"sync"
	"syscall"
	"time"

	"github.com/flotti97/e-ink_frame/internal/atomicfile"
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
	// Location is where the Image was taken, written onto its Slide; "" for none.
	Location string `json:"location,omitempty"`
}

// Library is a directory of Images and their Slides. It is safe for
// concurrent use, also from several processes sharing the directory.
type Library struct {
	dir string
	mu  sync.Mutex // serialises goroutines; lock serialises processes
}

// Open uses dir as a Library, creating it if needed, and removes files left
// behind by an Add or write that never finished.
func Open(dir string) (*Library, error) {
	for _, sub := range []string{"images", "slides"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	l := &Library{dir: dir}
	unlock, err := l.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := l.removeOrphans(); err != nil {
		return nil, err
	}
	return l, nil
}

// lock takes the Library for exclusive use, within this process and across
// processes, until the returned func is called.
func (l *Library) lock() (unlock func(), err error) {
	l.mu.Lock()
	f, err := os.OpenFile(filepath.Join(l.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err == nil {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != nil {
			f.Close()
		}
	}
	if err != nil {
		l.mu.Unlock()
		return nil, err
	}
	return func() {
		f.Close() // releases the flock
		l.mu.Unlock()
	}, nil
}

// removeOrphans deletes temporary files, and Images and Slides that have no
// Entry. The Entry is written last on Add and removed first on Delete, so a
// file without one belongs to an operation that was cut short.
func (l *Library) removeOrphans() error {
	for _, sub := range []string{"images", "slides"} {
		files, err := os.ReadDir(filepath.Join(l.dir, sub))
		if err != nil {
			return err
		}
		for _, f := range files {
			name := f.Name()
			if !strings.HasSuffix(name, ".tmp") {
				if strings.HasSuffix(name, ".json") {
					continue
				}
				id, _, _ := strings.Cut(name, ".")
				if _, err := os.Stat(l.entryPath(id)); err == nil {
					continue
				}
			}
			if err := os.Remove(filepath.Join(l.dir, sub, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
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
	// Rendering is slow, so it happens before taking the lock.
	var s bytes.Buffer
	if err := png.Encode(&s, slide.Render(img)); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	return l.store(name, data, s.Bytes())
}

// AddRendered is Add for an Image whose Slide, as PNG, was rendered
// elsewhere, such as in the browser. The Slide is checked, not trusted;
// the Image itself is only checked to be a JPEG or PNG.
func (l *Library) AddRendered(name string, r io.Reader, slidePNG []byte) (Entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := slide.CheckImage(data); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := slide.Check(slidePNG); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	return l.store(name, data, slidePNG)
}

// store keeps the Image data and its Slide s, unless the Image is there already.
func (l *Library) store(name string, data, s []byte) (Entry, error) {
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:8])
	ext := "png"
	if http.DetectContentType(data) == "image/jpeg" {
		ext = "jpg"
	}

	unlock, err := l.lock()
	if err != nil {
		return Entry{}, err
	}
	defer unlock()

	if e, err := l.get(id); err == nil {
		return e, nil
	}
	e := Entry{ID: id, Name: name, Ext: ext, Added: time.Now().UTC()}
	meta, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}

	if err := atomicfile.Write(l.SlidePath(id), s); err != nil {
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := atomicfile.Write(l.ImagePath(e), data); err != nil {
		os.Remove(l.SlidePath(id))
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := atomicfile.Write(l.entryPath(id), meta); err != nil {
		os.Remove(l.SlidePath(id))
		os.Remove(l.ImagePath(e))
		return Entry{}, fmt.Errorf("%s: %w", name, err)
	}
	return e, nil
}

// List returns every stored Image, oldest first. Entries that can't be read
// are left out rather than failing the whole listing.
func (l *Library) List() ([]Entry, error) {
	unlock, err := l.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	files, err := filepath.Glob(filepath.Join(l.dir, "images", "*.json"))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		e, err := l.get(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return a.Added.Compare(b.Added) })
	return entries, nil
}

// Delete removes the Image with this ID and its Slide.
func (l *Library) Delete(id string) error {
	unlock, err := l.lock()
	if err != nil {
		return err
	}
	defer unlock()

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

// SetLocation sets where the Image with this ID was taken.
func (l *Library) SetLocation(id, location string) error {
	unlock, err := l.lock()
	if err != nil {
		return err
	}
	defer unlock()

	e, err := l.get(id)
	if err != nil {
		return err
	}
	e.Location = location
	meta, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return atomicfile.Write(l.entryPath(id), meta)
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
