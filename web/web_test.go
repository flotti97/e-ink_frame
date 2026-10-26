package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/jpeg"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/flotti97/e-ink_frame/library"
	"github.com/flotti97/e-ink_frame/slide"
	"github.com/flotti97/e-ink_frame/slideshow"
	"github.com/flotti97/e-ink_frame/tag"
)

// fakeSlideshow records "show now" presses and the Clock settings.
type fakeSlideshow struct {
	current  string
	err      error
	shown    []string
	clock    slideshow.ClockSettings
	clockErr error
	tag      tag.Settings
	retagged chan string
}

func (f *fakeSlideshow) Retag(id string) error {
	if f.retagged != nil {
		f.retagged <- id
	}
	return nil
}

func (f *fakeSlideshow) TagSettings() tag.Settings { return f.tag }

func (f *fakeSlideshow) SetTag(t tag.Settings) error {
	f.tag = t
	return nil
}

func (f *fakeSlideshow) ClockSettings() slideshow.ClockSettings { return f.clock }

func (f *fakeSlideshow) SetClock(c slideshow.ClockSettings) error {
	if f.clockErr != nil {
		return f.clockErr
	}
	f.clock = c
	return nil
}

func (f *fakeSlideshow) ShowNow(id string) error {
	if f.err != nil {
		return f.err
	}
	f.shown = append(f.shown, id)
	f.current = id
	return nil
}

func (f *fakeSlideshow) Current() string { return f.current }

func newServer(t *testing.T) (*library.Library, http.Handler) {
	lib, _, h := newServerWithSlideshow(t)
	return lib, h
}

func newServerWithSlideshow(t *testing.T) (*library.Library, *fakeSlideshow, http.Handler) {
	t.Helper()
	lib, err := library.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	show := &fakeSlideshow{tag: tag.Default}
	return lib, show, New(lib, show, func(context.Context, float64, float64) (string, error) { return "Nowhere", nil })
}

func pngBytes(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 40, 30))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// upload posts files (name → contents) as the upload form would.
func upload(t *testing.T, h http.Handler, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for name, data := range files {
		part, err := w.CreateFormFile("images", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestUploadAddsImageToGalleryWithItsSlide(t *testing.T) {
	lib, h := newServer(t)

	rec := upload(t, h, map[string][]byte{"cat.png": pngBytes(t, 80), "dog.png": pngBytes(t, 160)})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("upload answered %d → %q, want a redirect to the gallery", rec.Code, rec.Header().Get("Location"))
	}

	entries, _ := lib.List()
	if len(entries) != 2 {
		t.Fatalf("library holds %d Images, want 2", len(entries))
	}
	page := get(h, "/").Body.String()
	for _, e := range entries {
		if !strings.Contains(page, e.Name) || !strings.Contains(page, "/slides/"+e.ID+".png") {
			t.Errorf("gallery is missing %s (%s)", e.Name, e.ID)
		}
		if _, err := os.Stat(lib.SlidePath(e.ID)); err != nil {
			t.Errorf("no Slide for %s: %v", e.Name, err)
		}
		slide := get(h, "/slides/"+e.ID+".png")
		if slide.Code != http.StatusOK || slide.Header().Get("Content-Type") != "image/png" {
			t.Errorf("Slide %s served as %d %s", e.ID, slide.Code, slide.Header().Get("Content-Type"))
		}
	}
}

func TestDeleteRemovesImageAndSlide(t *testing.T) {
	lib, h := newServer(t)
	upload(t, h, map[string][]byte{"cat.png": pngBytes(t, 80)})
	entries, _ := lib.List()
	id := entries[0].ID

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/delete/"+id, nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete answered %d", rec.Code)
	}
	if entries, _ := lib.List(); len(entries) != 0 {
		t.Errorf("Image still listed after delete")
	}
	if _, err := os.Stat(lib.SlidePath(id)); !os.IsNotExist(err) {
		t.Errorf("Slide still on disk after delete")
	}
	if strings.Contains(get(h, "/").Body.String(), id) {
		t.Errorf("gallery still shows the deleted Image")
	}
}

func TestInvalidUploadShowsErrorAndKeepsThePageUsable(t *testing.T) {
	lib, h := newServer(t)
	upload(t, h, map[string][]byte{"keep.png": pngBytes(t, 30)})

	rec := upload(t, h, map[string][]byte{"notes.txt": []byte("hello"), "good.png": pngBytes(t, 200)})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid upload answered %d, want 400", rec.Code)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "notes.txt") || !strings.Contains(page, "unsupported") {
		t.Errorf("error page does not explain what went wrong:\n%s", page)
	}
	// Still the full page: gallery and upload form are there.
	if !strings.Contains(page, "keep.png") || !strings.Contains(page, `action="/upload"`) {
		t.Errorf("error page lost the gallery or the form")
	}
	// The valid file in the same upload was still added.
	if entries, _ := lib.List(); len(entries) != 2 {
		t.Errorf("library holds %d Images, want 2", len(entries))
	}
}

func TestUploadWithoutFilesIsAnError(t *testing.T) {
	_, h := newServer(t)
	rec := upload(t, h, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Choose") {
		t.Errorf("empty upload answered %d", rec.Code)
	}
}

func TestDeleteUnknownImageIsNotFound(t *testing.T) {
	_, h := newServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/delete/nope", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `action="/upload"`) {
		t.Errorf("unknown delete answered %d without the page", rec.Code)
	}
}

func TestNamesAreEscaped(t *testing.T) {
	_, h := newServer(t)
	upload(t, h, map[string][]byte{"<script>x</script>.png": pngBytes(t, 50)})
	if strings.Contains(get(h, "/").Body.String(), "<script>x") {
		t.Error("Image name rendered as HTML")
	}
}

func TestPageWorksOnAPhone(t *testing.T) {
	_, h := newServer(t)
	page := get(h, "/").Body.String()
	for _, want := range []string{`name="viewport"`, `accept="image/jpeg,image/png"`, "multiple"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
}

func TestSlidePathCannotEscape(t *testing.T) {
	_, h := newServer(t)
	for _, p := range []string{"/slides/..%2Fimages%2Fx.png", "/slides/nope.png"} {
		rec := get(h, p)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", p, rec.Code)
		}
		io.Copy(io.Discard, rec.Body)
	}
}

func TestCrossSiteRequestsAreRefused(t *testing.T) {
	lib, h := newServer(t)
	upload(t, h, map[string][]byte{"cat.png": pngBytes(t, 80)})
	entries, _ := lib.List()

	req := httptest.NewRequest(http.MethodPost, "/delete/"+entries[0].ID, nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site delete answered %d, want 403", rec.Code)
	}
	if entries, _ := lib.List(); len(entries) != 1 {
		t.Error("cross-site request deleted an Image")
	}
}

func post(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

func TestShowNowPutsThatSlideUp(t *testing.T) {
	lib, show, h := newServerWithSlideshow(t)
	upload(t, h, map[string][]byte{"cat.png": pngBytes(t, 80), "dog.png": pngBytes(t, 160)})
	entries, _ := lib.List()
	id := entries[1].ID

	if !strings.Contains(get(h, "/").Body.String(), `action="/show/`+id+`"`) {
		t.Fatal("gallery has no show-now button for the Image")
	}
	rec := post(h, "/show/"+id)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("show now answered %d, want a redirect to the gallery", rec.Code)
	}
	if len(show.shown) != 1 || show.shown[0] != id {
		t.Errorf("Slideshow was asked to show %v, want [%s]", show.shown, id)
	}
}

func TestShowNowErrorsKeepThePage(t *testing.T) {
	_, show, h := newServerWithSlideshow(t)
	show.err = fmt.Errorf("x: %w", library.ErrNotFound)
	if rec := post(h, "/show/x"); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `action="/upload"`) {
		t.Errorf("unknown Image answered %d", rec.Code)
	}
	show.err = errors.New("panel unplugged")
	rec := post(h, "/show/x")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "panel unplugged") {
		t.Errorf("failed Refresh answered %d without the reason", rec.Code)
	}
}

func TestGalleryHighlightsTheCurrentSlide(t *testing.T) {
	lib, show, h := newServerWithSlideshow(t)
	upload(t, h, map[string][]byte{"cat.png": pngBytes(t, 80), "dog.png": pngBytes(t, 160)})
	entries, _ := lib.List()
	show.current = entries[0].ID

	page := get(h, "/").Body.String()
	if n := strings.Count(page, `aria-current="true"`); n != 1 {
		t.Fatalf("%d Images marked current, want 1", n)
	}
	marked := page[strings.Index(page, `aria-current="true"`):]
	marked = marked[:strings.Index(marked, "</li>")]
	if !strings.Contains(marked, entries[0].ID) || strings.Contains(marked, entries[1].ID) {
		t.Errorf("the wrong Image is marked current")
	}
}

func TestClockSettingsAreShownAndSaved(t *testing.T) {
	_, show, h := newServerWithSlideshow(t)
	show.clock = slideshow.ClockSettings{On: true, Step: 20, Style: "ink"}
	page := get(h, "/").Body.String()
	for _, want := range []string{`action="/clock"`, `value="true" checked`, `name="step"`, `value="20"`, `value="ink" selected`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	// An unchecked box sends nothing.
	show.tag.On = false // its switch is on the page too
	if rec := postForm(h, "/clock", "step=40&style=chalk"); rec.Code != http.StatusSeeOther {
		t.Fatalf("saving answered %d", rec.Code)
	}
	if want := (slideshow.ClockSettings{Step: 40, Style: "chalk"}); show.clock != want {
		t.Errorf("clock = %+v, want %+v", show.clock, want)
	}
	page = get(h, "/").Body.String()
	if strings.Contains(page, "checked") || !strings.Contains(page, `value="chalk" selected`) {
		t.Error("page does not show the saved settings")
	}
}

func TestBadClockSettingsKeepThePage(t *testing.T) {
	_, show, h := newServerWithSlideshow(t)
	for _, form := range []string{"on=true&step=x&style=ink", "on=true&step=1&style=ink", "on=true&step=20&style=crayon"} {
		if rec := postForm(h, "/clock", form); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `action="/upload"`) {
			t.Errorf("%s answered %d", form, rec.Code)
		}
	}
	if show.clock != (slideshow.ClockSettings{}) {
		t.Errorf("bad settings reached the Slideshow: %+v", show.clock)
	}
	show.clockErr = errors.New("disk full")
	if rec := postForm(h, "/clock", "step=20&style=ink"); rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "disk full") {
		t.Errorf("failed save answered %d without the reason", rec.Code)
	}
}

func postForm(h http.Handler, path, form string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUploadAskingForJSONReportsProblemsAsJSON(t *testing.T) {
	_, h := newServer(t)

	for _, tc := range []struct {
		name string
		data []byte
		code int
	}{{"good.png", pngBytes(t, 90), http.StatusNoContent}, {"notes.txt", []byte("hello"), http.StatusBadRequest}} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("images", tc.name)
		part.Write(tc.data)
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/upload", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != tc.code {
			t.Errorf("%s answered %d, want %d", tc.name, rec.Code, tc.code)
		}
		if tc.code == http.StatusBadRequest {
			var got struct{ Problems []string }
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Problems) != 1 || !strings.Contains(got.Problems[0], tc.name) {
				t.Errorf("%s answered %q, want JSON problems naming the file", tc.name, rec.Body.String())
			}
		}
	}
}

func TestUploadUsesTheBrowsersSlideOrRendersItsOwn(t *testing.T) {
	lib, h := newServer(t)
	var good bytes.Buffer
	png.Encode(&good, slide.Render(image.NewGray(image.Rect(0, 0, 40, 30))))

	for _, tc := range []struct {
		name   string
		slide  []byte
		wantAs []byte // nil: whatever the Frame renders
	}{{"browser.png", good.Bytes(), good.Bytes()}, {"forged.png", []byte("not a Slide"), nil}} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("images", tc.name)
		part.Write(pngBytes(t, uint8(len(tc.name))))
		part, _ = mw.CreateFormFile("slide", "slide.png")
		part.Write(tc.slide)
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/upload", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s answered %d %s", tc.name, rec.Code, rec.Body)
		}
	}

	entries, _ := lib.List()
	if len(entries) != 2 {
		t.Fatalf("library holds %d Images, want 2", len(entries))
	}
	for _, e := range entries {
		got, _ := os.ReadFile(lib.SlidePath(e.ID))
		if err := slide.Check(got); err != nil {
			t.Errorf("%s: stored Slide fails the check: %v", e.Name, err)
		}
		if e.Name == "browser.png" && !bytes.Equal(got, good.Bytes()) {
			t.Errorf("browser.png: the browser's Slide was not kept")
		}
	}
}

func TestTagSettingsAreShownAndSaved(t *testing.T) {
	_, show, h := newServerWithSlideshow(t)
	body := get(h, "/").Body.String()
	for _, want := range []string{`action="/tags"`, `value="cormorant" selected`, `value="top-left" selected`, `name="size"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %s", want)
		}
	}
	if rec := postForm(h, "/tags", "on=true&auto=true&position=bottom-right&font=caveat&size=90"); rec.Code != http.StatusSeeOther {
		t.Fatalf("saving answered %d", rec.Code)
	}
	if want := (tag.Settings{On: true, Auto: true, Position: "bottom-right", Font: "caveat", Size: 90}); show.tag != want {
		t.Fatalf("saved %+v, want %+v", show.tag, want)
	}
	if rec := postForm(h, "/tags", "position=middle&font=caveat&size=90"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad position answered %d", rec.Code)
	}
}

func TestTagFontsAreServed(t *testing.T) {
	_, h := newServer(t)
	if rec := get(h, "/fonts/marker.ttf"); rec.Code != http.StatusOK || rec.Body.Len() < 1000 {
		t.Fatalf("font answered %d, %d bytes", rec.Code, rec.Body.Len())
	}
	if rec := get(h, "/fonts/nope.ttf"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown font answered %d", rec.Code)
	}
}

// withGPS puts an EXIF segment with a GPS position into a JPEG.
func withGPS(t *testing.T, img []byte) []byte {
	t.Helper()
	o := binary.LittleEndian
	x := o.AppendUint32([]byte("II*\x00"), 8)
	x = o.AppendUint16(x, 1)
	x = o.AppendUint16(x, 0x8825)
	x = o.AppendUint16(x, 4)
	x = o.AppendUint32(x, 1)
	x = o.AppendUint32(x, 26)
	x = o.AppendUint32(x, 0)
	x = o.AppendUint16(x, 4)
	for _, e := range [][3]uint32{{1, 2, 'N'}, {2, 5, 80}, {3, 2, 'E'}, {4, 5, 104}} {
		x = o.AppendUint16(x, uint16(e[0]))
		x = o.AppendUint16(x, uint16(e[1]))
		x = o.AppendUint32(x, map[uint32]uint32{2: 2, 5: 3}[e[1]])
		x = o.AppendUint32(x, e[2])
	}
	x = o.AppendUint32(x, 0)
	for _, v := range []uint32{47, 1, 33, 1, 45, 1, 13, 1, 38, 1, 51, 1} {
		x = o.AppendUint32(x, v)
	}
	seg := append([]byte("Exif\x00\x00"), x...)
	out := binary.BigEndian.AppendUint16([]byte{0xFF, 0xD8, 0xFF, 0xE1}, uint16(len(seg)+2))
	return append(append(out, seg...), img[2:]...)
}

func TestUploadsAreTaggedFromTheirGPSPosition(t *testing.T) {
	lib, show, h := newServerWithSlideshow(t)
	show.tag.Auto = true
	show.retagged = make(chan string, 1)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 40, 30)), nil); err != nil {
		t.Fatal(err)
	}
	if rec := upload(t, h, map[string][]byte{"hallstatt.jpg": withGPS(t, b.Bytes())}); rec.Code != http.StatusSeeOther {
		t.Fatalf("upload answered %d: %s", rec.Code, rec.Body)
	}
	id := <-show.retagged
	entries, _ := lib.List()
	if len(entries) != 1 || entries[0].ID != id || entries[0].Location != "Nowhere" {
		t.Fatalf("entries %+v", entries)
	}
}
