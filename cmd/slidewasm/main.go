//go:build js && wasm

// Command slidewasm renders Slides in the browser, so the Frame's small
// computer doesn't have to. The upload page loads it in a Web Worker; it
// defines renderSlide(bytes Uint8Array) returning the Slide as PNG bytes, or
// a string with the reason the Image can't be used. (A Go panic would end the
// program rather than throw.)
package main

import (
	"bytes"
	"image/png"
	"syscall/js"

	"github.com/flotti97/e-ink_frame/slide"
)

func main() {
	js.Global().Set("renderSlide", js.FuncOf(func(this js.Value, args []js.Value) (result any) {
		data := make([]byte, args[0].Length())
		js.CopyBytesToGo(data, args[0])
		img, err := slide.Decode(bytes.NewReader(data))
		if err == nil {
			var b bytes.Buffer
			if err = png.Encode(&b, slide.Render(img)); err == nil {
				out := js.Global().Get("Uint8Array").New(b.Len())
				js.CopyBytesToJS(out, b.Bytes())
				return out
			}
		}
		return err.Error()
	}))
	select {} // keep serving calls
}
