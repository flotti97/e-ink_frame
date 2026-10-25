// Package web serves the upload page: a gallery of the stored Images with an
// upload form, the Clock's settings, and on each Image a "show now" and a
// delete button. The Slide on the panel is highlighted.
package web

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/flotti97/e-ink_frame/library"
	"github.com/flotti97/e-ink_frame/slideshow"
)

// maxUpload caps one upload request. Phone photos are a few MB each.
const maxUpload = 200 << 20

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("page").Parse(pageHTML))

// Slideshow is the running Slideshow the page controls.
type Slideshow interface {
	ShowNow(id string) error
	Current() string
	ClockSettings() slideshow.ClockSettings
	SetClock(c slideshow.ClockSettings) error
}

type server struct {
	lib  *library.Library
	show Slideshow
}

// New returns the handler for the upload page.
func New(lib *library.Library, show Slideshow) http.Handler {
	s := &server{lib: lib, show: show}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.gallery)
	mux.HandleFunc("POST /upload", s.upload)
	mux.HandleFunc("POST /delete/{id}", s.delete)
	mux.HandleFunc("POST /show/{id}", s.showNow)
	mux.HandleFunc("POST /clock", s.setClock)
	mux.HandleFunc("GET /slides/{file}", s.slide)
	// There is no login, so refuse form posts that other websites make the
	// browser send to the Frame.
	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *server) gallery(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, nil)
}

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(8 << 20); err != nil { // the rest spills to disk; the Pi has 512 MB
		s.render(w, http.StatusBadRequest, []string{"Upload failed: " + err.Error()})
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		s.render(w, http.StatusBadRequest, []string{"Choose at least one JPEG or PNG Image."})
		return
	}
	var problems []string
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", fh.Filename, err))
			continue
		}
		_, err = s.lib.Add(fh.Filename, f)
		f.Close()
		if err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		s.render(w, http.StatusBadRequest, problems)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	}{problems, entries, s.show.Current(), s.show.ClockSettings(), slideshow.MinClockStep, slideshow.MaxClockStep}); err != nil {
		log.Printf("web: rendering page: %v", err)
	}
}
