package geotag

import (
	"context"
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// jpegWithGPS is the start of a JPEG whose EXIF puts it at 47°33'45"N 13°38'51"E.
func jpegWithGPS() []byte {
	o := binary.LittleEndian
	t := []byte("II*\x00")
	t = o.AppendUint32(t, 8)
	// IFD0 at 8: one entry, the GPS IFD pointer.
	t = o.AppendUint16(t, 1)
	t = o.AppendUint16(t, 0x8825)
	t = o.AppendUint16(t, 4)
	t = o.AppendUint32(t, 1)
	t = o.AppendUint32(t, 26)
	t = o.AppendUint32(t, 0)
	// GPS IFD at 26: four entries, rationals after it at 26+2+48+4 = 80.
	t = o.AppendUint16(t, 4)
	ascii := func(tag uint16, c byte) {
		t = o.AppendUint16(t, tag)
		t = o.AppendUint16(t, 2)
		t = o.AppendUint32(t, 2)
		t = append(t, c, 0, 0, 0)
	}
	rational := func(tag uint16, off uint32) {
		t = o.AppendUint16(t, tag)
		t = o.AppendUint16(t, 5)
		t = o.AppendUint32(t, 3)
		t = o.AppendUint32(t, off)
	}
	ascii(1, 'N')
	rational(2, 80)
	ascii(3, 'E')
	rational(4, 104)
	t = o.AppendUint32(t, 0)
	for _, v := range []uint32{47, 1, 33, 1, 45, 1, 13, 1, 38, 1, 51, 1} {
		t = o.AppendUint32(t, v)
	}
	seg := append([]byte("Exif\x00\x00"), t...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(seg)+2))
	return append(append(out, seg...), 0xFF, 0xD9)
}

func TestGPS(t *testing.T) {
	lat, lon, ok := GPS(jpegWithGPS())
	if !ok || math.Abs(lat-47.5625) > 1e-6 || math.Abs(lon-13.6475) > 1e-6 {
		t.Fatalf("got %v, %v, %v", lat, lon, ok)
	}
	if _, _, ok := GPS([]byte{0xFF, 0xD8, 0xFF, 0xD9}); ok {
		t.Fatal("found GPS in a JPEG without EXIF")
	}
}

func TestLocate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("lat") != "47.5625" || r.Header.Get("User-Agent") == "" {
			t.Errorf("request %s", r.URL)
		}
		w.Write([]byte(`{"address":{"village":"Hallstatt","county":"Gmunden","country":"Austria"}}`))
	}))
	defer srv.Close()
	got, err := (&Nominatim{URL: srv.URL}).Locate(context.Background(), 47.5625, 13.6475)
	if err != nil || got != "Hallstatt, Austria" {
		t.Fatalf("got %q, %v", got, err)
	}
}
