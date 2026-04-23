// Package usbserial provides a unified Go API for USB-to-serial
// adapters on Linux, macOS, and Windows.
//
// The core Port interface is the same across all chipsets and
// platforms. Behind the scenes:
//
//   - On Linux and macOS, supported chipsets are driven through
//     libusb via chipset-specific subpackages (e.g. cp210x, ftdi).
//     No vendor kernel driver is required.
//   - On Windows, where there's no practical userspace path to
//     USB devices without shipping a kernel driver, Port falls
//     through to the OS's COM port API via go.bug.st/serial.
//
// Consumers import usbserial plus any chipset subpackages they
// want registered:
//
//	import (
//	    "github.com/packetThrower/usbserial-go/usbserial"
//	    _ "github.com/packetThrower/usbserial-go/cp210x"
//	)
//
// Go's linker drops unused packages, so only registered chipsets
// contribute to the final binary size.
package usbserial

import (
	"io"
	"time"
)

// Port is an open serial connection over USB. All implementations
// must be safe for concurrent Read and Write from different
// goroutines; control-setting methods (SetBaudRate, SetFraming,
// etc.) are not required to be concurrent-safe and callers should
// serialize them externally.
type Port interface {
	io.ReadWriteCloser

	// SetBaudRate sets the line speed. Common values: 9600, 19200,
	// 38400, 57600, 115200. Chipset implementations should error
	// on speeds they can't synthesize accurately (some chipsets are
	// restricted to a discrete set).
	SetBaudRate(baud int) error

	// SetFraming sets the data bits, stop bits, and parity.
	SetFraming(f Framing) error

	// SetFlowControl sets RTS/CTS or XON/XOFF handshaking, or disables it.
	SetFlowControl(fc FlowControl) error

	// SetDTR drives the Data Terminal Ready control line.
	SetDTR(assert bool) error

	// SetRTS drives the Request To Send control line.
	SetRTS(assert bool) error

	// GetModemStatus reads the current state of the input control
	// lines (CTS, DSR, RI, DCD). Not all chipsets support all four.
	GetModemStatus() (ModemStatus, error)

	// SendBreak drives the TX line low for the given duration
	// (300 ms is a common value for Cisco ROMMON, Juniper diag,
	// and similar bootloader interrupts).
	SendBreak(duration time.Duration) error
}

// Framing describes the character frame on the serial line.
type Framing struct {
	DataBits int    // 5, 6, 7, or 8
	StopBits int    // 1 or 2; 15 is used to encode 1.5 stop bits on chipsets that support it
	Parity   Parity
}

// Parity selects parity-bit generation and checking.
type Parity int

const (
	ParityNone Parity = iota
	ParityOdd
	ParityEven
	ParityMark
	ParitySpace
)

// FlowControl selects the handshake mode.
type FlowControl int

const (
	FlowNone FlowControl = iota
	FlowRTSCTS
	FlowXONXOFF
)

// ModemStatus reports the state of input control lines at the
// moment GetModemStatus is called.
type ModemStatus struct {
	CTS bool // Clear To Send
	DSR bool // Data Set Ready
	RI  bool // Ring Indicator
	DCD bool // Data Carrier Detect
}
