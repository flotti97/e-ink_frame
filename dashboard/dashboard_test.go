package dashboard

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flotti97/e-ink_frame/slide"
)

func TestFetchFitsAWidePictureOnWhite(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 400, 100)) // black, wider than the panel
	var b bytes.Buffer
	png.Encode(&b, src)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b.Bytes()) }))
	defer srv.Close()

	g, err := Fetch(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if g.Bounds() != image.Rect(0, 0, slide.Width, slide.Height) {
		t.Fatalf("bounds %v", g.Bounds())
	}
	if c := g.GrayAt(slide.Width/2, 10); c != (color.Gray{0xFF}) {
		t.Errorf("top margin %v, want white", c)
	}
	if c := g.GrayAt(slide.Width/2, slide.Height/2); c != (color.Gray{0}) {
		t.Errorf("centre %v, want black", c)
	}
}

func TestFetchReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("want an error")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		s  Settings
		ok bool
	}{
		{Default, true},
		{Settings{On: true, URL: "http://ha.local:10000/lovelace/0", Interval: time.Minute}, true},
		{Settings{On: true, Interval: time.Minute}, false},
		{Settings{URL: "ftp://x", Interval: time.Minute}, false},
		{Settings{URL: "http://x", Interval: time.Second}, false},
	} {
		if err := tc.s.Validate(); (err == nil) != tc.ok {
			t.Errorf("%+v: err %v", tc.s, err)
		}
	}
}
