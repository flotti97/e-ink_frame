package slideshow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/flotti97/e-ink_frame/internal/atomicfile"
)

// Config holds the Frame's settings, read from a JSON file such as
//
//	{"interval": "45m", "library": "data", "state": "data/shuffle.json", "listen": ":8080", "vcom": -1.58,
//	 "clock": true, "clock_step": 20, "clock_style": "ink"}
type Config struct {
	Interval time.Duration // time between Slides
	Library  string        // Image library directory
	State    string        // where the Shuffle is saved
	Listen   string        // address of the upload page
	VCOM     float64       // panel VCOM in volts, from its flex cable; 0 means unset
	Clock    ClockSettings // how the time is written on every Slide
}

// Clock styles.
const (
	StyleInk   = "ink"   // black or white straight on the photo, in DU
	StyleChalk = "chalk" // white on a black band, in the faster A2
)

// ClockSettings is how the Clock writes the time.
type ClockSettings struct {
	On    bool
	Step  int    // pixels of pen travel per partial Refresh
	Style string // StyleInk or StyleChalk
}

// Limits of ClockSettings.Step.
const (
	MinClockStep = 5
	MaxClockStep = 200
)

// Validate reports settings the Clock can't write with.
func (c ClockSettings) Validate() error {
	if c.Step < MinClockStep || c.Step > MaxClockStep {
		return fmt.Errorf("clock step %d px out of range (%d..%d)", c.Step, MinClockStep, MaxClockStep)
	}
	if c.Style != StyleInk && c.Style != StyleChalk {
		return fmt.Errorf("clock style %q unknown (want %q or %q)", c.Style, StyleInk, StyleChalk)
	}
	return nil
}

// DefaultConfig is used for anything the config file leaves out.
var DefaultConfig = Config{
	Interval: 45 * time.Minute,
	Library:  "data",
	State:    "data/shuffle.json",
	Listen:   ":8080",
	Clock:    ClockSettings{On: true, Step: 20, Style: StyleInk},
}

// LoadConfig reads the config file at path. A missing file gives DefaultConfig.
func LoadConfig(path string) (Config, error) {
	c := DefaultConfig
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}

	var raw struct {
		Interval *string  `json:"interval"`
		Library  *string  `json:"library"`
		State    *string  `json:"state"`
		Listen   *string  `json:"listen"`
		VCOM     *float64 `json:"vcom"`
		Clock    *bool    `json:"clock"`
		Step     *int     `json:"clock_step"`
		Style    *string  `json:"clock_style"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if raw.Interval != nil {
		d, err := time.ParseDuration(*raw.Interval)
		if err != nil {
			return c, fmt.Errorf("%s: interval: %w", path, err)
		}
		if d <= 0 {
			return c, fmt.Errorf("%s: interval must be positive, got %s", path, d)
		}
		c.Interval = d
	}
	if raw.Library != nil {
		c.Library = *raw.Library
	}
	if raw.State != nil {
		c.State = *raw.State
	}
	if raw.Listen != nil {
		c.Listen = *raw.Listen
	}
	if raw.VCOM != nil {
		c.VCOM = *raw.VCOM
	}
	if raw.Clock != nil {
		c.Clock.On = *raw.Clock
	}
	if raw.Step != nil {
		c.Clock.Step = *raw.Step
	}
	if raw.Style != nil {
		c.Clock.Style = *raw.Style
	}
	if err := c.Clock.Validate(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// SaveClock sets the clock settings in the config file at path, keeping its
// other settings. A missing file is created.
func SaveClock(path string, c ClockSettings) error {
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	raw["clock"], _ = json.Marshal(c.On)
	raw["clock_step"], _ = json.Marshal(c.Step)
	raw["clock_style"], _ = json.Marshal(c.Style)
	out, err := json.MarshalIndent(raw, "", "\t")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(out, '\n'))
}
