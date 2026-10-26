// Renders Slides off the page's main thread with the WASM build of the slide
// package. Answers each {id, bytes} with {id, slide} (PNG bytes), {id, error}
// for an Image that can't be used, or {id, unavailable} if the WASM build
// couldn't be loaded, in which case the Frame renders instead.
const ready = (async () => {
  importScripts("wasm_exec.js");
  const go = new Go();
  const res = await fetch("slide.wasm");
  if (!res.ok) throw new Error(`slide.wasm: ${res.status}`);
  const { instance } = await WebAssembly.instantiateStreaming(res, go.importObject);
  go.run(instance);
})();

onmessage = async ({ data: { id, bytes } }) => {
  try {
    await ready;
  } catch (err) {
    postMessage({ id, unavailable: String(err) });
    return;
  }
  const out = renderSlide(bytes);
  if (typeof out === "string") {
    postMessage({ id, error: out });
  } else {
    postMessage({ id, slide: out }, [out.buffer]);
  }
};
