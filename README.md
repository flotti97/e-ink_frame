# E-Ink Frame

A picture frame that slowly rotates through your photos on a 10.3" e-paper panel. No backlight, no glare, and the last picture stays on screen even without power.

> **Status:** early development. The Slide preview, the Image library, the Slideshow on the real panel and the upload page with "show now" exist so far.

## Hardware

- [Waveshare 10.3inch e-Paper HAT](https://www.waveshare.com/wiki/10.3inch_e-Paper_HAT): 1872×1404, 16 gray levels, IT8951 controller, SPI
- Raspberry Pi Zero W (ARMv6, single core, 512 MB RAM, 2.4 GHz Wi-Fi)
- Always on mains power, landscape orientation

## How it works

1. Someone uploads a JPEG or PNG **Image** through a small web page on the Pi, reachable from the home network only.
2. The **Image** is processed once into a **Slide**: crop to fill with a saliency-based smart crop, then 16-level grayscale with Floyd–Steinberg dithering.
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
- Run the Slideshow. On the Pi it drives the panel, which needs `vcom` in the config (see below). On a PC, use a fake panel, which writes the current Slide to a PNG you can keep open in an image viewer:

  ```bash
  go run ./cmd/frame -fake-panel panel.png
  ```

  It also serves the upload page on port 8080: open `http://<pi-address>:8080` on a phone or PC on the home network to upload Images, see the gallery and delete Images. "Show now" puts an Image's Slide on the panel straight away; the next Slide follows a full interval later, and the round of the shuffle carries on without repeats. The Slide on the panel is highlighted. There is no login, so don't expose this port to the internet.

  Settings come from `frame.json` (pass another file with `-config`). Every field is optional:

  ```json
  {"interval": "45m", "library": "data", "state": "data/shuffle.json", "listen": ":8080", "vcom": -1.58}
  ```

  `vcom` is the value printed on the panel's flex cable. There's no default, because a wrong value can damage the panel.

  The shuffle is saved in `state` after every Slide, so a restart carries on with the same round.
- Put one Slide (a Slide PNG, such as one from `data/slides`) or a test card on the real panel, on the Pi. Pass the VCOM from the panel's flex cable:

  ```bash
  GOOS=linux GOARCH=arm GOARM=6 go build ./cmd/panel
  scp panel pi@<pi-address>:
  ssh pi@<pi-address> ./panel -vcom -1.58 -test-card
  ```

  The test card has 16 gray bars running from black on the left to white, above thin lines and a diagonal that show whether the pixel order is right.
- Run the tests with `go test ./...`.

## Install on the Pi

1. Set up Raspberry Pi OS on the Pi Zero W with SSH and Wi-Fi. Enable SPI (`dtparam=spi=on` in `/boot/config.txt`), and add the `pi` user to the `spi` and `gpio` groups.
2. Connect the panel before powering on, and set the HAT's DIP switch to SPI.
3. From this repo on your PC, build and install the service:

   ```bash
   deploy/deploy.sh pi@<pi-address>
   ```

   This cross-compiles one static ARMv6 binary and installs it under `~/frame` on the Pi. It also installs [deploy/frame.service](deploy/frame.service) as a systemd service that starts at boot and restarts 10 s after a crash.
4. On a first install the script creates `~/frame/frame.json` and stops. Set `vcom` there to the value on the panel's flex cable, then run `sudo systemctl enable --now frame` on the Pi.

To update, run `deploy/deploy.sh` again. It replaces the binary and the unit and restarts the service, and it keeps `frame.json` and the Image library in `~/frame/data`. Read the logs with `journalctl -u frame`.

## Panel notes

From the Waveshare wiki:

- Connect all cables before powering on. Hot plugging isn't supported.
- Set the VCOM value printed on the panel's flex cable. A wrong value can damage the panel over time.
- Set the DIP switch on the driver board to match the interface (SPI).
- Operating range is 0–50 °C, and the panel can't update in direct sunlight.
- Refresh at least every 24 hours to avoid burn-in.

What we found driving the IT8951 (`it8951/`) from Linux:

- Wiring: CS is CE0 (`/dev/spidev0.0`, toggled by the kernel), reset is GPIO 17 and HRDY is GPIO 24, both through sysfs. The `pi` user needs to be in the `spi` and `gpio` groups.
- spidev caps a transaction at `bufsiz` (4096 bytes by default). Pixel data goes out as many write-data transactions; the controller keeps loading the image across them until "load image end".
- The device-info command returns 20 words. Read them all, or the controller holds HRDY low and the next command times out.
- Writes run at 8 MHz, reads at 2 MHz.
- In 4 bpp mode each 16-bit word holds 4 pixels as the vendor demo packs them: pixels p0..p3 go out as the bytes `p2p3 p0p1`.
- In the landscape frame the panel scans upside down: rows must be sent bottom-up. The controller's rotate setting can't fix this, because it rotates but doesn't flip.
- A full GC16 Refresh of a whole Slide, including the transfer, takes about 5 s on a Pi Zero W.

## Project docs

- [CONTEXT.md](CONTEXT.md): domain vocabulary
- [docs/agents/](docs/agents/): issue tracker and domain-doc conventions for coding agents
