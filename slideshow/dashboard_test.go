package slideshow

import (
	"context"
	"image"
	"slices"
	"testing"
	"time"

	"github.com/flotti97/e-ink_frame/dashboard"
)

// dash is a fetched Dashboard, told apart from Slides by its first pixel.
func dash(context.Context, string) (*image.Gray, error) {
	g := image.NewGray(image.Rect(0, 0, 4, 3))
	g.Pix[0] = 200
	return g, nil
}

var dashOn = dashboard.Settings{On: true, URL: "http://ha.local/dash.png", Interval: 5 * time.Minute}

func TestDashboardIsFetchedEveryDashboardInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var waited []time.Duration
	show, panel := newShow(t, 3, func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		if len(waited) == 3 {
			cancel()
			return ctx.Err()
		}
		return nil
	})
	show.Dashboard, show.Fetch, show.Clock.On = dashOn, dash, true
	show.Run(ctx)

	// The first goes up with a full Refresh, the next ones without a flash.
	if !slices.Equal(panel.shown, []uint8{200}) || len(panel.updates) != 2 {
		t.Errorf("panel showed %v and %d updates, want the dashboard once and 2 updates", panel.shown, len(panel.updates))
	}
	if waited[0] != dashOn.Interval {
		t.Errorf("waited %s, want the dashboard interval", waited[0])
	}
	if show.Current() != "" {
		t.Errorf("Current() = %q while the dashboard is up", show.Current())
	}
}

func TestSetDashboardSwitchesAtOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var show *Slideshow
	var waits int
	show, panel := newShow(t, 3, func(wctx context.Context, d time.Duration) error {
		waits++
		switch waits {
		case 1:
			if err := show.SetDashboard(dashOn); err != nil {
				t.Error(err)
			}
		case 2:
			off := dashOn
			off.On = false
			if err := show.SetDashboard(off); err != nil {
				t.Error(err)
			}
		default:
			cancel()
			return ctx.Err()
		}
		<-wctx.Done()
		return wctx.Err()
	})
	show.Fetch = dash
	show.Run(ctx)

	if len(panel.shown) != 3 || panel.shown[1] != 200 || panel.shown[2] == 200 {
		t.Errorf("panel showed %v, want a Slide, the dashboard, a Slide", panel.shown)
	}
}

func TestSetDashboardRejectsAMissingURL(t *testing.T) {
	show, _ := newShow(t, 1, nil)
	if err := show.SetDashboard(dashboard.Settings{On: true, Interval: time.Minute}); err == nil {
		t.Fatal("want an error")
	}
}
