// Package dashboard fetches a Home Assistant dashboard as a picture, such as
// the PNG the Puppet or lovelace-kindle-screensaver add-on serves, and fits
// it to the panel.
package dashboard

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"io"
	"net/http"
	"net/url"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/flotti97/e-ink_frame/slide"
)

// Settings is whether the Frame shows the Dashboard instead of the
// Slideshow, where it gets it and how often.
type Settings struct {
	On       bool
	URL      string        // a JPEG or PNG of the dashboard
	Interval time.Duration // time between fetches
}

// Limits of Settings.Interval.
const (
	MinInterval = time.Minute
	MaxInterval = 24 * time.Hour
)

// Default is the Settings when the config leaves them out.
var Default = Settings{Interval: 5 * time.Minute}

// Validate reports Settings the Frame can't work with. The URL may be empty
// while the Dashboard is off.
func (s Settings) Validate() error {
	if s.Interval < MinInterval || s.Interval > MaxInterval {
		return fmt.Errorf("dashboard interval %s out of range (%s..%s)", s.Interval, MinInterval, MaxInterval)
	}
	if s.URL == "" {
		if s.On {
			return fmt.Errorf("dashboard URL missing")
		}
		return nil
	}
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("dashboard URL %q is not an http(s) address", s.URL)
	}
	return nil
}

// maxSize caps a downloaded dashboard picture.
const maxSize = 20 << 20

// Fetch downloads the picture at url and fits it to the panel.
func Fetch(ctx context.Context, client *http.Client, url string) (*image.Gray, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dashboard: %s", resp.Status)
	}
	img, err := slide.Decode(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, fmt.Errorf("dashboard: %w", err)
	}
	return Fit(img), nil
}

// Fit scales img to fit inside the panel without cropping, centred on
// white, in the panel's gray levels. A picture the panel's size is only toned.
func Fit(img image.Image) *image.Gray {
	b := img.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, slide.Width, slide.Height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	w, h := slide.Width, slide.Height
	if b.Dx()*slide.Height > b.Dy()*slide.Width {
		h = b.Dy() * slide.Width / b.Dx()
	} else {
		w = b.Dx() * slide.Height / b.Dy()
	}
	x, y := (slide.Width-w)/2, (slide.Height-h)/2
	dst := image.Rect(x, y, x+w, y+h)
	if dst.Size() == b.Size() {
		draw.Draw(canvas, dst, img, b.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(canvas, dst, img, b, xdraw.Src, nil)
	}
	return slide.Tone(canvas)
}
