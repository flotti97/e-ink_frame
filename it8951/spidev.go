package it8951

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// HAT wiring (BCM numbering). CS is CE0, driven by the kernel's spidev.
const (
	pinReset = 17
	pinHRDY  = 24
)

// Clock speeds. Reads are slower: the controller fails at write speed.
const (
	writeHz = 8_000_000
	readHz  = 2_000_000
)

// SPIDev is a Bus on Linux spidev and sysfs GPIO.
type SPIDev struct {
	f       *os.File
	maxLen  int
	hrdy    *os.File
	resetFn string
}

// spiIOCTransfer mirrors struct spi_ioc_transfer.
type spiIOCTransfer struct {
	txBuf, rxBuf uint64
	len, speedHz uint32
	delayUsecs   uint16
	bitsPerWord  uint8
	csChange     uint8
	txNbits      uint8
	rxNbits      uint8
	wordDelay    uint8
	pad          uint8
}

const spiIOCMessage1 = 0x40206B00 // SPI_IOC_MESSAGE(1)

// OpenSPIDev opens dev (for example /dev/spidev0.0) and the HAT's GPIO lines.
func OpenSPIDev(dev string) (*SPIDev, error) {
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	maxLen := 4096
	if b, err := os.ReadFile("/sys/module/spidev/parameters/bufsiz"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			maxLen = n
		}
	}
	reset, err := exportGPIO(pinReset, "out")
	if err != nil {
		f.Close()
		return nil, err
	}
	hrdy, err := exportGPIO(pinHRDY, "in")
	if err != nil {
		f.Close()
		return nil, err
	}
	h, err := os.Open(hrdy)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &SPIDev{f: f, maxLen: maxLen, hrdy: h, resetFn: reset}, nil
}

// Close releases the device.
func (d *SPIDev) Close() error {
	d.hrdy.Close()
	return d.f.Close()
}

// MaxTransfer is spidev's buffer size.
func (d *SPIDev) MaxTransfer() int { return d.maxLen }

// Transfer does one SPI transaction.
func (d *SPIDev) Transfer(w, r []byte) error {
	if len(w) == 0 || len(w) > d.maxLen {
		return fmt.Errorf("transfer of %d bytes outside 1..%d (spidev bufsiz)", len(w), d.maxLen)
	}
	t := spiIOCTransfer{
		txBuf:       uint64(uintptr(unsafe.Pointer(&w[0]))),
		len:         uint32(len(w)),
		speedHz:     writeHz,
		bitsPerWord: 8,
	}
	if r != nil {
		t.rxBuf = uint64(uintptr(unsafe.Pointer(&r[0])))
		t.speedHz = readHz
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.f.Fd(), spiIOCMessage1, uintptr(unsafe.Pointer(&t)))
	runtime.KeepAlive(w)
	runtime.KeepAlive(r)
	if errno != 0 {
		return fmt.Errorf("spi transfer: %w", errno)
	}
	return nil
}

// Ready reads HRDY.
func (d *SPIDev) Ready() (bool, error) {
	var b [1]byte
	if _, err := d.hrdy.ReadAt(b[:], 0); err != nil {
		return false, err
	}
	return b[0] == '1', nil
}

// Reset pulses reset low the way the vendor demo does.
func (d *SPIDev) Reset() error {
	for _, step := range []struct {
		v string
		d time.Duration
	}{{"1", 200 * time.Millisecond}, {"0", 10 * time.Millisecond}, {"1", 200 * time.Millisecond}} {
		if err := os.WriteFile(d.resetFn, []byte(step.v), 0); err != nil {
			return err
		}
		time.Sleep(step.d)
	}
	return nil
}

// exportGPIO exports pin through sysfs and returns its value file.
func exportGPIO(pin int, dir string) (string, error) {
	base := fmt.Sprintf("/sys/class/gpio/gpio%d", pin)
	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile("/sys/class/gpio/export", []byte(strconv.Itoa(pin)), 0); err != nil {
			return "", fmt.Errorf("export GPIO %d: %w", pin, err)
		}
	}
	// udev sets the permissions shortly after export.
	var err error
	for range 50 {
		if err = os.WriteFile(base+"/direction", []byte(dir), 0); err == nil {
			return base + "/value", nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "", fmt.Errorf("GPIO %d direction: %w", pin, err)
}
