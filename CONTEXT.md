# E-Ink Frame

A picture frame built around an e-paper panel that rotates through a collection of photos, slowly and without a backlight.

## Hardware

Raspberry Pi zero W. ssh pi@192.168.1.74

## Language

**Frame**:
The whole physical device: panel, computer and the software that drives them.
_Avoid_: Display, device, screen

**Image**:
A source photo as someone supplied it, before any processing.
_Avoid_: Photo, picture, original

**Slide**:
A display-ready version of an Image, sized and toned for the panel.
_Avoid_: Frame (reserved for the device), processed image, render

**Slideshow**:
The ordered rotation of Slides shown on the Frame, shuffled so every Slide appears once before any repeats.
_Avoid_: Playlist, carousel, gallery

**Refresh**:
One physical update of the panel. A _full_ Refresh redraws and clears the whole panel; a _partial_ Refresh updates only part of it.
_Avoid_: Redraw, update, flip

**Tag**:
An Image's location written onto its Slide when it is shown. Typed on the upload page or looked up from the Image's GPS position.
_Avoid_: Caption, label
