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
	if !c.Clock {
		t.Error("clock off by default")
	}
	c, err = LoadConfig(writeConfig(t, `{"clock": false}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Clock {
		t.Error(`"clock": false ignored`)
	}
}
