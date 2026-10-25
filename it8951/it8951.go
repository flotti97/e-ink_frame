// Package it8951 drives the IT8951 e-paper controller of the Waveshare 10.3"
// e-Paper HAT over SPI. The protocol is ported from the Waveshare vendor demo.
package it8951

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"time"
)

// Bus is the wiring between the host and the controller.
type Bus interface {
	// Transfer does one SPI transaction with CS held low throughout. When r
	// is not nil it has len(w) bytes and receives what the controller sends.
	Transfer(w, r []byte) error
	// Ready reports the HRDY line: true when the controller takes a transaction.
	Ready() (bool, error)
	// Reset pulses the controller's reset line.
	Reset() error
	// MaxTransfer is the largest transaction Transfer accepts, in bytes.
	MaxTransfer() int
}

// Transaction preambles.
const (
	preCommand = 0x6000
	preWrite   = 0x0000
	preRead    = 0x1000
)

// Commands.
const (
	cmdRun         = 0x0001
	cmdSleep       = 0x0003
	cmdRegRead     = 0x0010
	cmdRegWrite    = 0x0011
	cmdLoadImgArea = 0x0021
	cmdLoadImgEnd  = 0x0022
	cmdDisplayArea = 0x0034
	cmdVCOM        = 0x0039
	cmdGetDevInfo  = 0x0302
)

// Registers.
const (
	regI80CPCR = 0x0004 // packed write enable
	regLISAR   = 0x0208 // target image buffer address, low word; high word at +2
	regLUTAFSR = 0x1224 // non-zero while a Refresh is running
)

// Mode picks the waveform of a Refresh: how fast it is, how many gray levels
// it reaches and how much it flashes. Numbers are the 10.3" panel's.
type Mode uint16

const (
	ModeInit Mode = 0 // clears to white with heavy flashing
	ModeDU   Mode = 1 // fast, black and white only; other pixels keep their gray
	ModeGC16 Mode = 2 // 16 gray levels, flashes
	ModeGL16 Mode = 3 // 16 gray levels, no flash, more ghosting
	ModeA2   Mode = 6 // fastest, but only between black and white: leaves grays as they are
)

const (
	bpp4         = 2
	littleEndian = 0

	readyTimeout   = 5 * time.Second
	refreshTimeout = 30 * time.Second
)

// Panel is an IT8951 with its panel attached.
type Panel struct {
	Width, Height int

	bus     Bus
	bufAddr uint32
}

// Open resets the controller, reads the panel size and sets VCOM, in volts
// as printed on the panel's flex cable (for example -1.58).
func Open(bus Bus, vcom float64) (*Panel, error) {
	if vcom >= 0 || vcom < -5 {
		return nil, fmt.Errorf("VCOM %v V out of range (want -5..0)", vcom)
	}
	p := &Panel{bus: bus}
	if err := bus.Reset(); err != nil {
		return nil, err
	}
	// The controller stays busy until all 20 info words are read.
	info, err := p.query(cmdGetDevInfo, 20)
	if err != nil {
		return nil, fmt.Errorf("get device info: %w", err)
	}
	p.Width, p.Height = int(info[0]), int(info[1])
	p.bufAddr = uint32(info[2]) | uint32(info[3])<<16
	if p.Width == 0 || p.Height == 0 || p.Width == 0xFFFF {
		return nil, fmt.Errorf("controller reports a %dx%d panel; check wiring and the DIP switch", p.Width, p.Height)
	}
	if err := p.writeReg(regI80CPCR, 1); err != nil {
		return nil, err
	}
	mv := uint16(math.Round(-vcom * 1000))
	if err := p.command(cmdVCOM, 1, mv); err != nil {
		return nil, fmt.Errorf("set VCOM: %w", err)
	}
	return p, nil
}

// Show puts s on the panel with one full Refresh, then puts the panel to
// sleep, also when the Refresh fails.
func (p *Panel) Show(s *image.Gray) error {
	b := s.Bounds()
	if b.Dx() != p.Width || b.Dy() != p.Height || b.Dx()%4 != 0 {
		return fmt.Errorf("Slide is %dx%d, panel is %dx%d", b.Dx(), b.Dy(), p.Width, p.Height)
	}
	if err := p.refresh(s); err != nil {
		p.Sleep()
		return err
	}
	return p.Sleep()
}

// Update refreshes area r of the panel from s with mode m. r's left edge and
// width must be multiples of 4. The panel stays awake until Sleep.
func (p *Panel) Update(s *image.Gray, r image.Rectangle, m Mode) error {
	if err := p.Load(s, r); err != nil {
		return err
	}
	return p.Display(r, m)
}

// Sleep puts the panel to sleep; Load wakes it again.
func (p *Panel) Sleep() error {
	return p.command(cmdSleep)
}

func (p *Panel) refresh(s *image.Gray) error {
	r := image.Rect(0, 0, p.Width, p.Height)
	if err := p.Load(s, r); err != nil {
		return err
	}
	return p.Display(r, ModeGC16)
}

// Load sends area r of s, in Slide coordinates, to the controller's buffer
// without changing the panel. r's left edge and width must be multiples of 4.
func (p *Panel) Load(s *image.Gray, r image.Rectangle) error {
	if r.Min.X%4 != 0 || r.Dx()%4 != 0 || r.Empty() ||
		!r.In(image.Rect(0, 0, p.Width, p.Height)) || !r.In(s.Bounds()) {
		return fmt.Errorf("area %v not 4-aligned or outside the %dx%d panel", r, p.Width, p.Height)
	}
	if err := p.command(cmdRun); err != nil {
		return err
	}
	if err := p.writeReg(regLISAR+2, uint16(p.bufAddr>>16)); err != nil {
		return err
	}
	if err := p.writeReg(regLISAR, uint16(p.bufAddr)); err != nil {
		return err
	}
	x, y, w, h := p.area(r)
	if err := p.command(cmdLoadImgArea, littleEndian<<8|bpp4<<4, x, y, w, h); err != nil {
		return err
	}
	if err := p.writePixels(pack(s.SubImage(r).(*image.Gray))); err != nil {
		return fmt.Errorf("write pixels: %w", err)
	}
	return p.command(cmdLoadImgEnd)
}

// Display refreshes area r of the panel, in Slide coordinates, from the
// controller's buffer with mode m and waits until the Refresh is done.
func (p *Panel) Display(r image.Rectangle, m Mode) error {
	if err := p.waitRefresh(); err != nil {
		return err
	}
	x, y, w, h := p.area(r)
	if err := p.command(cmdDisplayArea, x, y, w, h, uint16(m)); err != nil {
		return err
	}
	return p.waitRefresh()
}

// area converts r to controller coordinates, where rows run bottom-up.
func (p *Panel) area(r image.Rectangle) (x, y, w, h uint16) {
	return uint16(r.Min.X), uint16(p.Height - r.Max.Y), uint16(r.Dx()), uint16(r.Dy())
}

// pack turns s into 4-bit pixels, 0 black to 15 white, laid out the way the
// vendor demo sends them: four pixels to a little-endian word, sent high
// byte first. Rows go bottom-up: the panel scans upside down compared with
// the frame, and the controller can rotate but not flip.
func pack(s *image.Gray) []byte {
	b := s.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()/2)
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		row := s.Pix[s.PixOffset(b.Min.X, y):]
		for x := 0; x+3 < b.Dx(); x += 4 {
			out = append(out,
				row[x+2]&0xF0|row[x+3]>>4,
				row[x]&0xF0|row[x+1]>>4)
		}
	}
	return out
}

func (p *Panel) writePixels(data []byte) error {
	chunk := (p.bus.MaxTransfer() - 2) &^ 1
	buf := make([]byte, 0, chunk+2)
	for len(data) > 0 {
		n := min(chunk, len(data))
		buf = binary.BigEndian.AppendUint16(buf[:0], preWrite)
		buf = append(buf, data[:n]...)
		if err := p.transfer(buf, nil); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (p *Panel) waitRefresh() error {
	deadline := time.Now().Add(refreshTimeout)
	for {
		v, err := p.readReg(regLUTAFSR)
		if err != nil {
			return err
		}
		if v == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for the Refresh to finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (p *Panel) writeReg(addr, v uint16) error {
	return p.command(cmdRegWrite, addr, v)
}

func (p *Panel) readReg(addr uint16) (uint16, error) {
	if err := p.command(cmdRegRead, addr); err != nil {
		return 0, err
	}
	v, err := p.read(1)
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

// query sends cmd and reads n words back.
func (p *Panel) query(cmd uint16, n int) ([]uint16, error) {
	if err := p.command(cmd); err != nil {
		return nil, err
	}
	return p.read(n)
}

// command sends cmd, then each argument as its own data write.
func (p *Panel) command(cmd uint16, args ...uint16) error {
	if err := p.transfer(words(preCommand, cmd), nil); err != nil {
		return err
	}
	for _, a := range args {
		if err := p.transfer(words(preWrite, a), nil); err != nil {
			return err
		}
	}
	return nil
}

// read reads n words: preamble, one dummy word, then the data.
func (p *Panel) read(n int) ([]uint16, error) {
	w := make([]byte, 4+2*n)
	binary.BigEndian.PutUint16(w, preRead)
	r := make([]byte, len(w))
	if err := p.transfer(w, r); err != nil {
		return nil, err
	}
	out := make([]uint16, n)
	for i := range out {
		out[i] = binary.BigEndian.Uint16(r[4+2*i:])
	}
	return out, nil
}

func (p *Panel) transfer(w, r []byte) error {
	if err := p.waitReady(); err != nil {
		return err
	}
	return p.bus.Transfer(w, r)
}

func (p *Panel) waitReady() error {
	deadline := time.Now().Add(readyTimeout)
	for {
		ok, err := p.bus.Ready()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for HRDY")
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func words(ws ...uint16) []byte {
	b := make([]byte, 0, 2*len(ws))
	for _, w := range ws {
		b = binary.BigEndian.AppendUint16(b, w)
	}
	return b
}
