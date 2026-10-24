package library

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/flotti97/e-ink_frame/slide"
)

func testImage(shade uint8) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	img.Set(10, 10, color.Black)
	return img
}

func pngBytes(t *testing.T, shade uint8) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, testImage(shade)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T, shade uint8) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, testImage(shade), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func open(t *testing.T) *Library {
	t.Helper()
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestAddStoresImageAndSlide(t *testing.T) {
	lib := open(t)
	for name, data := range map[string][]byte{"a.jpg": jpegBytes(t, 50), "b.png": pngBytes(t, 200)} {
		e, err := lib.Add(name, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if e.Name != name {
			t.Errorf("entry name %q, want %q", e.Name, name)
		}

		kept, err := os.ReadFile(lib.ImagePath(e))
		if err != nil || !bytes.Equal(kept, data) {
			t.Errorf("%s: Image not kept byte-for-byte (err %v)", name, err)
		}

		f, err := os.Open(lib.SlidePath(e.ID))
		if err != nil {
			t.Fatalf("%s: no Slide: %v", name, err)
		}
		s, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if s.Bounds() != image.Rect(0, 0, slide.Width, slide.Height) {
			t.Errorf("%s: Slide is %v", name, s.Bounds())
		}
	}
}

func TestListShowsEveryStoredImageInOrderAdded(t *testing.T) {
	lib := open(t)
	var want []string
	for i, shade := range []uint8{10, 90, 170} {
		e, err := lib.Add(string(rune('a'+i))+".png", bytes.NewReader(pngBytes(t, shade)))
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, e.ID)
	}

	// A reopened Library sees the same Images.
	reopened, err := Open(lib.dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d Images, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("entry %d is %s, want %s", i, got[i].ID, want[i])
		}
	}
}

func TestAddingTheSameImageTwiceKeepsOneCopy(t *testing.T) {
	lib := open(t)
	data := pngBytes(t, 120)
	a, _ := lib.Add("one.png", bytes.NewReader(data))
	b, err := lib.Add("two.png", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Errorf("same bytes got IDs %s and %s", a.ID, b.ID)
	}
	if list, _ := lib.List(); len(list) != 1 {
		t.Errorf("listed %d Images, want 1", len(list))
	}
}

func TestDeleteRemovesImageAndSlide(t *testing.T) {
	lib := open(t)
	keep, _ := lib.Add("keep.png", bytes.NewReader(pngBytes(t, 30)))
	gone, _ := lib.Add("gone.jpg", bytes.NewReader(jpegBytes(t, 220)))

	if err := lib.Delete(gone.ID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{lib.ImagePath(gone), lib.SlidePath(gone.ID)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists after Delete", p)
		}
	}
	list, _ := lib.List()
	if len(list) != 1 || list[0].ID != keep.ID {
		t.Errorf("after Delete list is %v, want only %s", list, keep.ID)
	}
	if _, err := os.Stat(lib.SlidePath(keep.ID)); err != nil {
		t.Errorf("other Slide was touched: %v", err)
	}
}

func TestDeleteUnknownImage(t *testing.T) {
	if err := open(t).Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestAddRejectsOtherFormatsAndLeavesNoTrace(t *testing.T) {
	lib := open(t)
	for name, data := range map[string][]byte{
		"x.gif":     []byte("GIF89a\x01\x00\x01\x00"),
		"notes.txt": []byte("hello"),
		"cut.png":   pngBytes(t, 1)[:40],
	} {
		_, err := lib.Add(name, bytes.NewReader(data))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v, want a message naming the file", name, err)
		}
	}
	if list, _ := lib.List(); len(list) != 0 {
		t.Errorf("rejected files were listed: %v", list)
	}
	for _, sub := range []string{"images", "slides"} {
		if files, _ := os.ReadDir(lib.dir + "/" + sub); len(files) != 0 {
			t.Errorf("%s/ has leftovers: %v", sub, files)
		}
	}
}

func TestListSkipsCorruptEntries(t *testing.T) {
	lib := open(t)
	good, _ := lib.Add("good.png", bytes.NewReader(pngBytes(t, 60)))
	if err := os.WriteFile(lib.entryPath("bad0000000000000"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := lib.List()
	if err != nil {
		t.Fatalf("one corrupt entry broke List: %v", err)
	}
	if len(list) != 1 || list[0].ID != good.ID {
		t.Errorf("list is %v, want only %s", list, good.ID)
	}
}

func TestOpenRemovesOrphanedFiles(t *testing.T) {
	lib := open(t)
	good, _ := lib.Add("good.png", bytes.NewReader(pngBytes(t, 60)))
	orphans := []string{
		lib.SlidePath("orphan0000000000"),                        // Add crashed after the Slide
		lib.ImagePath(Entry{ID: "orphan0000000000", Ext: "jpg"}), // ... or after the Image
		lib.SlidePath(good.ID) + ".123.tmp",                      // a write that never finished
	}
	for _, p := range orphans {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Open(lib.dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range orphans {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("orphan %s survived Open", p)
		}
	}
	for _, p := range []string{lib.ImagePath(good), lib.SlidePath(good.ID), lib.entryPath(good.ID)} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("Open removed a stored file: %v", err)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	lib := open(t)
	const n = 6
	var wg sync.WaitGroup
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		data := pngBytes(t, uint8(20+i*30))
		wg.Add(2)
		go func() {
			defer wg.Done()
			e, err := lib.Add("x.png", bytes.NewReader(data))
			if err != nil {
				t.Error(err)
				return
			}
			ids <- e.ID
		}()
		go func() {
			defer wg.Done()
			if _, err := lib.List(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(ids)

	// Delete half while listing and re-adding the same bytes concurrently.
	var all []string
	for id := range ids {
		all = append(all, id)
	}
	for _, id := range all[:n/2] {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := lib.Delete(id); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := lib.List(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	list, _ := lib.List()
	if len(list) != n-n/2 {
		t.Errorf("listed %d Images, want %d", len(list), n-n/2)
	}
	for _, e := range list {
		if _, err := os.Stat(lib.SlidePath(e.ID)); err != nil {
			t.Errorf("listed Image %s has no Slide", e.ID)
		}
	}
}
