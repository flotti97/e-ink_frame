// Command frame runs the Slideshow.
//
//	frame [-config frame.json] -fake-panel panel.png
//
// Until the real panel driver exists, -fake-panel is required: each Slide is
// written to that PNG instead of the e-paper panel.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"

	"github.com/flotti97/e-ink_frame/library"
	"github.com/flotti97/e-ink_frame/slideshow"
)

func main() {
	configPath := flag.String("config", "frame.json", "config file (missing means defaults)")
	fakePanel := flag.String("fake-panel", "", "write each Slide to this PNG instead of the panel")
	flag.Parse()
	if *fakePanel == "" {
		fmt.Fprintln(os.Stderr, "frame: -fake-panel is required until the real panel is supported")
		os.Exit(2)
	}

	logger := log.New(os.Stderr, "frame: ", log.LstdFlags)
	if err := run(*configPath, *fakePanel, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatal(err)
	}
}

func run(configPath, fakePanel string, logger *log.Logger) error {
	cfg, err := slideshow.LoadConfig(configPath)
	if err != nil {
		return err
	}
	lib, err := library.Open(cfg.Library)
	if err != nil {
		return err
	}
	shuffle, err := slideshow.LoadShuffle(cfg.State)
	if errors.Is(err, slideshow.ErrCorruptState) {
		logger.Printf("starting a fresh shuffle: %v", err)
	} else if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Printf("Slideshow every %s from %s", cfg.Interval, cfg.Library)
	show := &slideshow.Slideshow{
		Library:   lib,
		Panel:     slideshow.FakePanel{Path: fakePanel},
		Shuffle:   shuffle,
		StatePath: cfg.State,
		Interval:  cfg.Interval,
		Rand:      rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		Log:       logger,
	}
	return show.Run(ctx)
}
