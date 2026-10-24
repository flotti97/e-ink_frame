# E-Ink Frame

A picture frame that slowly rotates through your photos on a 10.3" e-paper panel. No backlight, no glare, and the last picture stays on screen even without power.

> **Status:** early development. The Slide preview, the Image library and the Slideshow (on a fake panel) exist so far.

## Hardware

- [Waveshare 10.3inch e-Paper HAT](https://www.waveshare.com/wiki/10.3inch_e-Paper_HAT): 1872×1404, 16 gray levels, IT8951 controller, SPI
- Raspberry Pi Zero W (ARMv6, single core, 512 MB RAM, 2.4 GHz Wi-Fi)
- Always on mains power, landscape orientation

## How it works

1. Someone uploads a JPEG or PNG **Image** through a small web page on the Pi, reachable from the home network only.
2. The **Image** is processed once into a **Slide**: crop to fill with a saliency-based smart crop, then plain 16-level grayscale with no dithering.
3. The **Slideshow** shows one **Slide** at a time, every 45 minutes by default. The order is shuffled, and every **Slide** is shown once before any repeats.
4. Each change is a full **Refresh** of the panel. The panel sleeps between Refreshes, and the last **Slide** stays visible if Wi-Fi or the service fails.

The upload page is a gallery of Images with delete buttons, a "show now" button on each, and the current **Slide** highlighted.

The terms **Frame**, **Image**, **Slide**, **Slideshow** and **Refresh** are defined in [CONTEXT.md](CONTEXT.md).

## Stack

- Go, cross-compiled for ARMv6 (`GOARM=6`) into a single static binary
- A custom IT8951 driver over SPI, ported from the Waveshare vendor demo
- Standard-library `net/http` for the upload page
- Runs as a systemd service that restarts on failure
- Settings such as the Slideshow interval live in a config file

## Development

- Develop on your PC. A PC-side preview renders a **Slide** as a PNG, so you don't need the panel:

  ```bash
  go run ./cmd/preview photo.jpg slide.png
  ```

  It accepts JPEG or PNG and writes a 1872×1404, 16-level grayscale PNG.
- Manage the Image library (stored under `./data` by default; change it with `-dir`):

  ```bash
  go run ./cmd/library add photo.jpg other.png
  go run ./cmd/library list
  go run ./cmd/library rm <id>
  ```

  Adding an Image keeps it unchanged and stores its Slide next to it. Removing it deletes both.
- Run the Slideshow on a fake panel, which writes the current Slide to a PNG you can keep open in an image viewer:

  ```bash
  go run ./cmd/frame -fake-panel panel.png
  ```

  Settings come from `frame.json` (pass another file with `-config`). Every field is optional:

  ```json
  {"interval": "45m", "library": "data", "state": "data/shuffle.json"}
  ```

  The shuffle is saved in `state` after every Slide, so a restart carries on with the same round.
- Run the tests with `go test ./...`.
- Deploy by copying the binary to the Pi and restarting the service.

## Panel notes

From the Waveshare wiki:

- Connect all cables before powering on. Hot plugging isn't supported.
- Set the VCOM value printed on the panel's flex cable. A wrong value can damage the panel over time.
- Set the DIP switch on the driver board to match the interface (SPI).
- Operating range is 0–50 °C, and the panel can't update in direct sunlight.
- Refresh at least every 24 hours to avoid burn-in.

## Project docs

- [CONTEXT.md](CONTEXT.md): domain vocabulary
- [docs/agents/](docs/agents/): issue tracker and domain-doc conventions for coding agents
