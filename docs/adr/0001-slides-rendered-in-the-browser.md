# Slides are rendered in the browser

Rendering a Slide (smart crop, rescale, dithering) takes the Pi Zero long enough to stall uploads. The upload page now renders each Slide in a Web Worker with the `slide` package compiled to WASM (`cmd/slidewasm`), and posts it with the Image. Compiling the same Go code, rather than porting it to JavaScript, keeps a single implementation.

The Frame doesn't trust a Slide it didn't make: `slide.Check` verifies size, grayscale and panel gray levels, which is cheap. A Slide that fails, or an upload without one (no JavaScript, no WASM build, an old browser), is rendered on the Frame as before.

The WASM build (~3 MB) is embedded in the Frame binary by `go generate ./web` and only fetched when someone uploads.
