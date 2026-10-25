package slideshow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frame.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigDefaultsTo45Minutes(t *testing.T) {
	c, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Interval != 45*time.Minute {
		t.Errorf("default interval %v, want 45m", c.Interval)
	}
	if c.Listen != ":8080" {
		t.Errorf("default listen address %q, want :8080", c.Listen)
	}
	if c.Library == "" || c.State == "" {
		t.Errorf("defaults missing paths: %+v", c)
	}
}

func TestConfigReadsInterval(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, `{"interval": "1h30m", "library": "/srv/frame", "listen": "192.168.1.5:80"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Interval != 90*time.Minute || c.Library != "/srv/frame" || c.Listen != "192.168.1.5:80" {
		t.Errorf("got %+v", c)
	}
	if c.State == "" {
		t.Error("unset field lost its default")
	}
}

func TestConfigRejectsBadValues(t *testing.T) {
	for _, body := range []string{
		`{"interval": "soon"}`,
		`{"interval": "0s"}`,
		`{"interval": "-5m"}`,
		`{"intervall": "5m"}`,
		`{not json`,
	} {
		path := writeConfig(t, body)
		if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: err = %v, want an error naming the file", body, err)
		}
	}
}

func TestConfigReadsVCOM(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, `{"vcom": -1.58}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.VCOM != -1.58 {
		t.Errorf("vcom %v, want -1.58", c.VCOM)
	}
}

func TestConfigClockIsOnUnlessTurnedOff(t *testing.T) {
	c, err := LoadConfig(writeConfig(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Clock != (ClockSettings{On: true, Step: 20, Style: StyleInk}) {
		t.Errorf("default clock = %+v", c.Clock)
	}
	c, err = LoadConfig(writeConfig(t, `{"clock": false}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Clock.On {
		t.Error(`"clock": false ignored`)
	}
	c, err = LoadConfig(writeConfig(t, `{"clock_step": 40, "clock_style": "chalk"}`))
	if err != nil || c.Clock.Step != 40 || c.Clock.Style != StyleChalk {
		t.Errorf("clock = %+v, %v", c.Clock, err)
	}
	for _, bad := range []string{`{"clock_step": 0}`, `{"clock_style": "crayon"}`} {
		if _, err := LoadConfig(writeConfig(t, bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestSaveClockKeepsTheOtherSettings(t *testing.T) {
	path := writeConfig(t, `{"interval": "10m", "vcom": -1.58}`)
	want := ClockSettings{Step: 40, Style: StyleChalk}
	if err := SaveClock(path, want); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Clock != want || c.Interval != 10*time.Minute || c.VCOM != -1.58 {
		t.Errorf("config after saving = %+v", c)
	}

	missing := filepath.Join(t.TempDir(), "frame.json")
	if err := SaveClock(missing, DefaultConfig.Clock); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadConfig(missing); err != nil || c.Clock != DefaultConfig.Clock {
		t.Errorf("new config = %+v, %v", c, err)
	}
}
