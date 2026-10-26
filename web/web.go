// Package web serves the upload page: a gallery of the stored Images with an
// upload form, the Clock's settings, and on each Image a "show now" and a
// delete button. The Slide on the panel is highlighted.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/flotti97/e-ink_frame/geotag"
	"github.com/flotti97/e-ink_frame/library"
	"github.com/flotti97/e-ink_frame/slideshow"
	"github.com/flotti97/e-ink_frame/tag"
)

// maxUpload caps one upload request. Phone photos are a few MB each.
const maxUpload = 200 << 20

//go:embed page.html
var pageHTML string

// static holds the page's script for rendering Slides in the browser, and the
// WASM build of the slide package it runs, which go generate puts there.
// Without the WASM build the page leaves rendering to the Frame.
//
//go:generate sh -c "GOOS=js GOARCH=wasm go build -trimpath -ldflags=-s -o static/slide.wasm ../cmd/slidewasm && cp \"$(go env GOROOT)/lib/wasm/wasm_exec.js\" static/"
//go:embed static
var static embed.FS

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	// label turns a tag position like "top-left" into "Top left".
	"label": func(pos string) string {
		pos = strings.ReplaceAll(pos, "-", " ")
		return strings.ToUpper(pos[:1]) + pos[1:]
	},
}).Parse(pageHTML))

// Slideshow is the running Slideshow the page controls.
type Slideshow interface {
	ShowNow(id string) error
	Current() string
	ClockSettings() slideshow.ClockSettings
	SetClock(c slideshow.ClockSettings) error
	Retag(id string) error
	TagSettings() tag.Settings
	SetTag(t tag.Settings) error
}

// Locate names the place at a GPS position, such as geotag.Nominatim.Locate.
type Locate func(ctx context.Context, lat, lon float64) (string, error)

type server struct {
	lib     *library.Library
	show    Slideshow
	locate  Locate
	tagging atomic.Bool // a run of tagAll is going
}

// New returns the handler for the upload page. Images are tagged with the
// place locate names for their GPS position.
func New(lib *library.Library, show Slideshow, locate Locate) http.Handler {
	s := &server{lib: lib, show: show, locate: locate}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.gallery)
	mux.HandleFunc("POST /upload", s.upload)
	mux.HandleFunc("POST /delete/{id}", s.delete)
	mux.HandleFunc("POST /show/{id}", s.showNow)
	mux.HandleFunc("POST /clock", s.setClock)
	mux.HandleFunc("POST /location/{id}", s.setLocation)
	mux.HandleFunc("POST /tags", s.setTags)
	mux.HandleFunc("POST /tags/auto", s.autoTagAll)
	mux.HandleFunc("GET /fonts/{file}", s.font)
	mux.HandleFunc("GET /slides/{file}", s.slide)
	mux.Handle("GET /static/", http.FileServerFS(static))
	// There is no login, so refuse form posts that other websites make the
	// browser send to the Frame.
	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *server) gallery(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, nil)
}

// upload adds the posted Images. The page's script posts one file at a time
// and asks for JSON, so it can show progress and per-file errors without
// leaving the page; a plain form post gets the page back. The script also
// sends the Slide it rendered, if it could; a Slide that fails the check is
// rendered again here.
func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	wantJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	fail := func(problems []string) {
		if wantJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(struct {
				Problems []string `json:"problems"`
			}{problems})
			return
		}
		s.render(w, http.StatusBadRequest, problems)
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(8 << 20); err != nil { // the rest spills to disk; the Pi has 512 MB
		fail([]string{"Upload failed: " + err.Error()})
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		fail([]string{"Choose at least one JPEG or PNG Image."})
		return
	}
	var problems []string
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", fh.Filename, err))
			continue
		}
		e, err := s.add(fh.Filename, f, r.MultipartForm.File["slide"], len(files))
		f.Close()
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if s.show.TagSettings().Auto && e.Location == "" {
			go s.autoTag(e)
		}
	}
	switch {
	case len(problems) > 0:
		fail(problems)
	case wantJSON:
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// add stores one Image, with the Slide the browser rendered for it if there
// is one; only a single-Image upload can carry one.
func (s *server) add(name string, f io.ReadSeeker, slides []*multipart.FileHeader, images int) (library.Entry, error) {
	if len(slides) == 1 && images == 1 {
		e, err := s.addRendered(name, f, slides[0])
		if err == nil {
			return e, nil
		}
		log.Printf("web: rendering %s here, the browser's Slide won't do: %v", name, err)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return library.Entry{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	return s.lib.Add(name, f)
}

func (s *server) addRendered(name string, f io.Reader, fh *multipart.FileHeader) (library.Entry, error) {
	sf, err := fh.Open()
	if err != nil {
		return library.Entry{}, err
	}
	defer sf.Close()
	png, err := io.ReadAll(sf)
	if err != nil {
		return library.Entry{}, err
	}
	return s.lib.AddRendered(name, f, png)
}

func (s *server) delete(w http.ResponseWriter, r *http.Request) {
	err := s.lib.Delete(r.PathValue("id"))
	switch {
	case errors.Is(err, library.ErrNotFound):
		s.render(w, http.StatusNotFound, []string{"That Image is already gone."})
	case err != nil:
		s.render(w, http.StatusInternalServerError, []string{"Delete failed: " + err.Error()})
	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) showNow(w http.ResponseWriter, r *http.Request) {
	err := s.show.ShowNow(r.PathValue("id"))
	switch {
	case errors.Is(err, library.ErrNotFound):
		s.render(w, http.StatusNotFound, []string{"That Image is gone."})
	case err != nil:
		s.render(w, http.StatusInternalServerError, []string{"Show now failed: " + err.Error()})
	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) setClock(w http.ResponseWriter, r *http.Request) {
	step, err := strconv.Atoi(r.FormValue("step"))
	if err != nil {
		s.render(w, http.StatusBadRequest, []string{"Step size must be a whole number of pixels."})
		return
	}
	c := slideshow.ClockSettings{On: r.FormValue("on") == "true", Step: step, Style: r.FormValue("style")}
	if err := c.Validate(); err != nil {
		s.render(w, http.StatusBadRequest, []string{"Clock settings not saved: " + err.Error()})
		return
	}
	if err := s.show.SetClock(c); err != nil {
		s.render(w, http.StatusInternalServerError, []string{"Saving the clock settings failed: " + err.Error()})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// maxLocation caps a location's length in characters, so it fits on the Slide.
const maxLocation = 40

func (s *server) setLocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	loc := strings.Join(strings.Fields(r.FormValue("location")), " ")
	if n := len([]rune(loc)); n > maxLocation {
		s.render(w, http.StatusBadRequest, []string{fmt.Sprintf("Location is %d characters, keep it to %d.", n, maxLocation)})
		return
	}
	err := s.lib.SetLocation(id, loc)
	switch {
	case errors.Is(err, library.ErrNotFound):
		s.render(w, http.StatusNotFound, []string{"That Image is gone."})
		return
	case err != nil:
		s.render(w, http.StatusInternalServerError, []string{"Saving the location failed: " + err.Error()})
		return
	}
	go s.retag(id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// retag shows the Slide with this ID again if it is on the panel. It waits
// for a Refresh, so it runs in the background.
func (s *server) retag(id string) {
	if err := s.show.Retag(id); err != nil {
		log.Printf("web: retagging %s: %v", id, err)
	}
}

func (s *server) setTags(w http.ResponseWriter, r *http.Request) {
	size, err := strconv.Atoi(r.FormValue("size"))
	if err != nil {
		s.render(w, http.StatusBadRequest, []string{"Tag size must be a whole number of pixels."})
		return
	}
	t := tag.Settings{
		On: r.FormValue("on") == "true", Auto: r.FormValue("auto") == "true",
		Position: r.FormValue("position"), Font: r.FormValue("font"), Size: size,
	}
	if err := t.Validate(); err != nil {
		s.render(w, http.StatusBadRequest, []string{"Tag settings not saved: " + err.Error()})
		return
	}
	if err := s.show.SetTag(t); err != nil {
		s.render(w, http.StatusInternalServerError, []string{"Saving the tag settings failed: " + err.Error()})
		return
	}
	if cur := s.show.Current(); cur != "" {
		go s.retag(cur)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// autoTagAll tags every untagged Image that has a GPS position, in the
// background, one at a time.
func (s *server) autoTagAll(w http.ResponseWriter, r *http.Request) {
	entries, err := s.lib.List()
	if err != nil {
		s.render(w, http.StatusInternalServerError, []string{"Could not list Images: " + err.Error()})
		return
	}
	if s.tagging.CompareAndSwap(false, true) {
		go func() {
			defer s.tagging.Store(false)
			for _, e := range entries {
				if e.Location == "" {
					s.autoTag(e)
				}
			}
		}()
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// autoTag sets the location of e to the place its GPS position names, if it
// has one.
func (s *server) autoTag(e library.Entry) {
	data, err := os.ReadFile(s.lib.ImagePath(e))
	if err != nil {
		log.Printf("web: auto tag %s: %v", e.Name, err)
		return
	}
	lat, lon, ok := geotag.GPS(data)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	place, err := s.locate(ctx, lat, lon)
	if err != nil {
		log.Printf("web: auto tag %s at %.4f,%.4f: %v", e.Name, lat, lon, err)
		return
	}
	if len([]rune(place)) > maxLocation {
		place = string([]rune(place)[:maxLocation])
	}
	if err := s.lib.SetLocation(e.ID, place); err != nil {
		log.Printf("web: auto tag %s: %v", e.Name, err)
		return
	}
	log.Printf("web: tagged %s %q", e.Name, place)
	s.retag(e.ID)
}

// font serves a tag Font, so the page can show what it looks like.
func (s *server) font(w http.ResponseWriter, r *http.Request) {
	key, _ := strings.CutSuffix(r.PathValue("file"), ".ttf")
	for _, f := range tag.Fonts {
		if f.Key == key {
			w.Header().Set("Content-Type", "font/ttf")
			w.Header().Set("Cache-Control", "max-age=86400")
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(f.TTF()))
			return
		}
	}
	http.NotFound(w, r)
}

func (s *server) slide(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("file"), ".png")
	entries, err := s.lib.List()
	if !ok || err != nil || !slices.ContainsFunc(entries, func(e library.Entry) bool { return e.ID == id }) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache") // revalidate; a Slide may be re-rendered under the same ID
	http.ServeFile(w, r, s.lib.SlidePath(id))
}

// render writes the whole page, with any errors shown above the gallery, so
// the page stays usable whatever went wrong.
func (s *server) render(w http.ResponseWriter, status int, problems []string) {
	entries, err := s.lib.List()
	if err != nil {
		problems = append(problems, "Could not list Images: "+err.Error())
	}
	slices.Reverse(entries) // newest first
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Execute(w, struct {
		Problems []string
		Entries  []library.Entry
		Current  string
		Clock    slideshow.ClockSettings
		MinStep  int
		MaxStep  int
		MaxLoc   int
		Tag      tag.Settings
		Fonts    []tag.Font
		Pos      []string
		MinSize  int
		MaxSize  int
		Tagging  bool
	}{problems, entries, s.show.Current(), s.show.ClockSettings(), slideshow.MinClockStep, slideshow.MaxClockStep, maxLocation,
		s.show.TagSettings(), tag.Fonts, tag.Positions, tag.MinSize, tag.MaxSize, s.tagging.Load()}); err != nil {
		log.Printf("web: rendering page: %v", err)
	}
}
