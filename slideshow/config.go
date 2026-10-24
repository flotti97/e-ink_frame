package slideshow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Config holds the Frame's settings, read from a JSON file such as
//
//	{"interval": "45m", "library": "data", "state": "data/shuffle.json", "listen": ":8080", "vcom": -1.58}
type Config struct {
	Interval time.Duration // time between Slides
	Library  string        // Image library directory
	State    string        // where the Shuffle is saved
	Listen   string        // address of the upload page
	VCOM     float64       // panel VCOM in volts, from its flex cable; 0 means unset
}

// DefaultConfig is used for anything the config file leaves out.
var DefaultConfig = Config{
	Interval: 45 * time.Minute,
	Library:  "data",
	State:    "data/shuffle.json",
	Listen:   ":8080",
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
	return c, nil
}
