// Package geotag finds where a photo was taken: its GPS position from the
// EXIF data, named by reverse geocoding with OpenStreetMap's Nominatim.
package geotag

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GPS returns the position a JPEG's EXIF data records, in degrees. ok is
// false when it has none or it can't be read.
func GPS(data []byte) (lat, lon float64, ok bool) {
	tiff := exif(data)
	if len(tiff) < 8 {
		return 0, 0, false
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, 0, false
	}
	ifd0 := entries(tiff, order, int(order.Uint32(tiff[4:])))
	gpsAt, found := ifd0[0x8825]
	if !found {
		return 0, 0, false
	}
	gps := entries(tiff, order, int(order.Uint32(gpsAt[8:])))
	lat, okLat := degrees(tiff, order, gps[1], gps[2], 'S')
	lon, okLon := degrees(tiff, order, gps[3], gps[4], 'W')
	if !okLat || !okLon || (lat == 0 && lon == 0) {
		return 0, 0, false
	}
	return lat, lon, true
}

// exif returns the TIFF block of a JPEG's EXIF segment, or nil.
func exif(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		return nil
	}
	for i := 2; i+4 <= len(data) && data[i] == 0xFF; {
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		end := i + 2 + int(binary.BigEndian.Uint16(data[i+2:]))
		if end > len(data) {
			break
		}
		if marker == 0xE1 && bytes.HasPrefix(data[i+4:end], []byte("Exif\x00\x00")) {
			return data[i+10 : end]
		}
		i = end
	}
	return nil
}

// entries reads the IFD at offset off into its 12-byte entries by tag.
func entries(tiff []byte, order binary.ByteOrder, off int) map[uint16][]byte {
	out := map[uint16][]byte{}
	if off < 0 || off+2 > len(tiff) {
		return out
	}
	n := int(order.Uint16(tiff[off:]))
	for e := range n {
		at := off + 2 + e*12
		if at+12 > len(tiff) {
			break
		}
		out[order.Uint16(tiff[at:])] = tiff[at : at+12]
	}
	return out
}

// degrees reads a GPS coordinate: ref an ASCII entry, val three RATIONALs
// of degrees, minutes and seconds. neg is the ref letter that makes it
// negative.
func degrees(tiff []byte, order binary.ByteOrder, ref, val []byte, neg byte) (float64, bool) {
	if ref == nil || val == nil || order.Uint16(val[2:]) != 5 || order.Uint32(val[4:]) != 3 {
		return 0, false
	}
	off := int(order.Uint32(val[8:]))
	if off < 0 || off+24 > len(tiff) {
		return 0, false
	}
	var d float64
	for i, unit := range []float64{1, 60, 3600} {
		num, den := order.Uint32(tiff[off+i*8:]), order.Uint32(tiff[off+i*8+4:])
		if den == 0 {
			if num == 0 {
				continue
			}
			return 0, false
		}
		d += float64(num) / float64(den) / unit
	}
	if ref[8] == neg {
		d = -d
	}
	return d, true
}

// Nominatim names places with OpenStreetMap's Nominatim service, keeping to
// its limit of one request a second.
type Nominatim struct {
	URL    string // "" means the public service
	Client *http.Client

	mu   sync.Mutex
	last time.Time
}

// ErrNoPlace is returned for positions with no named place, such as at sea.
var ErrNoPlace = errors.New("no place there")

// Locate names the place at lat, lon as "town, country".
func (n *Nominatim) Locate(ctx context.Context, lat, lon float64) (string, error) {
	n.mu.Lock()
	if wait := time.Until(n.last.Add(time.Second)); wait > 0 {
		time.Sleep(wait)
	}
	n.last = time.Now()
	n.mu.Unlock()

	base := n.URL
	if base == "" {
		base = "https://nominatim.openstreetmap.org/reverse"
	}
	q := url.Values{
		"format": {"jsonv2"}, "lat": {fmt.Sprint(lat)}, "lon": {fmt.Sprint(lon)},
		"zoom": {"10"}, "accept-language": {"en"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "e-ink_frame/1 (github.com/flotti97/e-ink_frame)")
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nominatim answered %s", res.Status)
	}
	var body struct {
		Address map[string]string `json:"address"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	return name(body.Address)
}

// name picks the town and country out of a Nominatim address.
func name(a map[string]string) (string, error) {
	var parts []string
	for _, k := range []string{"city", "town", "village", "municipality", "county", "state"} {
		if v := a[k]; v != "" {
			parts = append(parts, v)
			break
		}
	}
	if c := a["country"]; c != "" {
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return "", ErrNoPlace
	}
	return strings.Join(parts, ", "), nil
}
