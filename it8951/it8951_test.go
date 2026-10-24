package it8951

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"slices"
	"testing"
)

// fakeBus records SPI transactions and answers reads like a minimal IT8951.
type fakeBus struct {
	txs    [][]byte // bytes written, one entry per transaction
	resets int
	maxLen int
	// failData fails every pixel write.
	failData bool
}

func (b *fakeBus) Reset() error         { b.resets++; return nil }
func (b *fakeBus) Ready() (bool, error) { return true, nil }
func (b *fakeBus) MaxTransfer() int     { return b.maxLen }
func (b *fakeBus) Transfer(w, r []byte) error {
	if b.failData && len(w) > 4 && binary.BigEndian.Uint16(w) == preWrite {
		return errors.New("bus failure")
	}
	b.txs = append(b.txs, bytes.Clone(w))
	if r == nil {
		return nil
	}
	// Answer reads with the words the last command asked for.
	switch b.lastCommand() {
	case cmdGetDevInfo:
		info := []uint16{1872, 1404, 0x36E0, 0x0012}
		for i, v := range info {
			binary.BigEndian.PutUint16(r[4+2*i:], v)
		}
	}
	// Register reads (LUT busy) answer 0, which r already holds.
	return nil
}

func (b *fakeBus) lastCommand() uint16 {
	for i := len(b.txs) - 1; i >= 0; i-- {
		if binary.BigEndian.Uint16(b.txs[i]) == preCommand {
			return binary.BigEndian.Uint16(b.txs[i][2:])
		}
	}
	return 0
}

// commands returns every command sent, in order.
func (b *fakeBus) commands() []uint16 {
	var cs []uint16
	for _, tx := range b.txs {
		if binary.BigEndian.Uint16(tx) == preCommand {
			cs = append(cs, binary.BigEndian.Uint16(tx[2:]))
		}
	}
	return cs
}

func TestOpenReadsDeviceInfoAndSetsVCOM(t *testing.T) {
	bus := &fakeBus{maxLen: 4096}
	p, err := Open(bus, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 1872 || p.Height != 1404 || p.bufAddr != 0x001236E0 {
		t.Errorf("info = %dx%d @%#x", p.Width, p.Height, p.bufAddr)
	}
	if bus.resets != 1 {
		t.Errorf("resets = %d, want 1", bus.resets)
	}
	// VCOM is sent as millivolts, positive.
	var found bool
	for i, tx := range bus.txs {
		if bytes.Equal(tx, []byte{0x60, 0x00, 0x00, 0x39}) {
			found = bytes.Equal(bus.txs[i+1], []byte{0, 0, 0, 1}) &&
				bytes.Equal(bus.txs[i+2], []byte{0, 0, 0x06, 0x2C})
		}
	}
	if !found {
		t.Error("VCOM 1580 not set")
	}
}

func TestOpenRejectsImplausibleVCOM(t *testing.T) {
	for _, v := range []float64{0, 1.5, -5.1} {
		if _, err := Open(&fakeBus{maxLen: 4096}, v); err == nil {
			t.Errorf("VCOM %v accepted", v)
		}
	}
}

func TestPackPutsFourPixelsPerWordInControllerOrder(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 4, 1))
	for x, v := range []uint8{0x00, 0x55, 0xAA, 0xFF} {
		img.SetGray(x, 0, color.Gray{v})
	}
	// Pixels p0..p3 = 0,5,A,F go out as bytes (p2<<4|p3), (p0<<4|p1):
	// the vendor demo's little-endian word, sent high byte first.
	got := pack(img)
	if want := []byte{0xAF, 0x05}; !bytes.Equal(got, want) {
		t.Errorf("pack = % X, want % X", got, want)
	}
}

func TestPackSendsRowsBottomUp(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 4, 2))
	copy(img.Pix[4:], []uint8{0xFF, 0xFF, 0xFF, 0xFF}) // bottom row white
	if got, want := pack(img), []byte{0xFF, 0xFF, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("pack = % X, want % X", got, want)
	}
}

func TestShowWritesWholeSlideInChunksThenRefreshesAndSleeps(t *testing.T) {
	bus := &fakeBus{maxLen: 4096}
	p, err := Open(bus, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	bus.txs = nil
	img := image.NewGray(image.Rect(0, 0, 1872, 1404))
	if err := p.Show(img); err != nil {
		t.Fatal(err)
	}

	var pixels int
	for _, tx := range bus.txs {
		if len(tx) > bus.maxLen {
			t.Fatalf("transfer of %d bytes exceeds %d", len(tx), bus.maxLen)
		}
		if len(tx) > 4 && binary.BigEndian.Uint16(tx) == preWrite {
			pixels += (len(tx) - 2) * 2
		}
	}
	if pixels != 1872*1404 {
		t.Errorf("wrote %d pixels, want %d", pixels, 1872*1404)
	}

	cs := bus.commands()
	want := []uint16{cmdRun, cmdRegWrite, cmdRegWrite, cmdLoadImgArea, cmdLoadImgEnd, cmdRegRead, cmdDisplayArea, cmdRegRead, cmdSleep}
	if !slices.Equal(cs, want) {
		t.Errorf("commands = %#x, want %#x", cs, want)
	}
}

func TestShowRejectsWrongSize(t *testing.T) {
	p, err := Open(&fakeBus{maxLen: 4096}, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Show(image.NewGray(image.Rect(0, 0, 100, 100))); err == nil {
		t.Error("wrong-size Slide accepted")
	}
}

func TestShowSleepsEvenWhenTheRefreshFails(t *testing.T) {
	bus := &fakeBus{maxLen: 4096}
	p, err := Open(bus, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	bus.failData = true
	if err := p.Show(image.NewGray(image.Rect(0, 0, 1872, 1404))); err == nil {
		t.Fatal("Show succeeded on a failing bus")
	}
	if cs := bus.commands(); cs[len(cs)-1] != cmdSleep {
		t.Errorf("last command %#x, want sleep", cs[len(cs)-1])
	}
}

func TestLoadAndDisplayAddressAreaBottomUp(t *testing.T) {
	bus := &fakeBus{maxLen: 4096}
	p, err := Open(bus, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	s := image.NewGray(image.Rect(0, 0, p.Width, p.Height))
	r := image.Rect(8, 100, 72, 164)
	bus.txs = nil
	if err := p.Load(s, r); err != nil {
		t.Fatal(err)
	}
	if err := p.Display(r, ModeA2); err != nil {
		t.Fatal(err)
	}
	// The area's top row in the Slide is its bottom row on the controller.
	area := words(preWrite, 8, preWrite, 1404-164, preWrite, 64, preWrite, 64)
	var args [][]byte
	for i, tx := range bus.txs {
		switch {
		case bytes.Equal(tx, words(preCommand, cmdLoadImgArea)): // after the pixel format
			args = append(args, slices.Concat(bus.txs[i+2:i+6]...))
		case bytes.Equal(tx, words(preCommand, cmdDisplayArea)): // before the mode
			args = append(args, slices.Concat(bus.txs[i+1:i+5]...))
		}
	}
	if len(args) != 2 || !bytes.Equal(args[0], area) || !bytes.Equal(args[1], area) {
		t.Errorf("area args = % X, want % X twice", args, area)
	}
	// 64x64 pixels at 4 bits.
	var n int
	for _, tx := range bus.txs {
		if len(tx) > 4 && binary.BigEndian.Uint16(tx) == preWrite {
			n += len(tx) - 2
		}
	}
	if n != 64*64/2 {
		t.Errorf("sent %d pixel bytes, want %d", n, 64*64/2)
	}
}

func TestLoadRejectsUnalignedArea(t *testing.T) {
	p, err := Open(&fakeBus{maxLen: 4096}, -1.58)
	if err != nil {
		t.Fatal(err)
	}
	s := image.NewGray(image.Rect(0, 0, p.Width, p.Height))
	for _, r := range []image.Rectangle{image.Rect(2, 0, 10, 8), image.Rect(0, 0, 6, 8), image.Rect(1868, 0, 1876, 8)} {
		if err := p.Load(s, r); err == nil {
			t.Errorf("area %v accepted", r)
		}
	}
}
