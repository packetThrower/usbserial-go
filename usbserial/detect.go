package usbserial

import (
	"fmt"
	"sync"
)

// Chipset names the protocol family a device speaks. Matches the
// subpackage name that implements it.
type Chipset string

const (
	ChipsetCP210x Chipset = "cp210x"
	ChipsetFTDI   Chipset = "ftdi"
	ChipsetCH341  Chipset = "ch341"
	ChipsetPL2303 Chipset = "pl2303"
)

// Device describes one attached USB-serial adapter, as returned by
// List. The fields are the ones a UI would want to show a user when
// picking a port.
type Device struct {
	// Chipset identifies which protocol family the adapter speaks,
	// and (on Linux/macOS) which subpackage will drive it.
	Chipset Chipset

	// VendorID and ProductID are the USB VID:PID, mainly useful for
	// diagnostics and for logs.
	VendorID  uint16
	ProductID uint16

	// Serial is the USB iSerial descriptor value, or empty if the
	// device didn't provide one. CP210x adapters typically have one
	// burned in at manufacture; CH340 clones often don't.
	Serial string

	// Path is a platform-appropriate identifier for this device:
	//   - On Linux / macOS, a URI like "usb:bus=001:addr=004" — the
	//     actual serial port device path only exists once the OS
	//     vendor driver claims the device. libusb doesn't create one.
	//   - On Windows, the traditional "COM3" / "COM42" device name
	//     from the OS's vendor driver.
	Path string

	// Driver names the concrete implementation that will handle
	// Open() calls for this device. Set by the subpackage at
	// registration time.
	Driver Driver
}

// Driver is the chipset-specific entry point the core package uses
// to open devices. Each chipset subpackage registers one at init()
// time via Register.
type Driver interface {
	// Name is the user-facing chipset name (matches Chipset).
	Name() Chipset

	// Matches reports whether this driver handles a given USB
	// vendor + product ID. Implementations typically consult a
	// hardcoded VID/PID table lifted from the chipset's datasheet.
	Matches(vid, pid uint16) bool

	// Open claims the device (on Linux/macOS via libusb, on
	// Windows via the OS COM port) and returns a Port ready for
	// serial traffic.
	Open(d Device) (Port, error)
}

var (
	registryMu sync.RWMutex
	drivers    []Driver
)

// Register adds a chipset driver to the registry. Each chipset
// subpackage calls this from its own init() function.
func Register(drv Driver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	drivers = append(drivers, drv)
}

// List returns every attached USB-serial adapter that a registered
// driver knows how to handle. On Linux and macOS it enumerates the
// USB bus via libusb. On Windows it enumerates COM ports via the
// OS and matches them against registered VID/PID tables.
func List() ([]Device, error) {
	return platformList()
}

// Open claims the device and returns a Port. Shortcut for calling
// d.Driver.Open(d) directly — included so callers don't have to
// dereference Driver manually.
func Open(d Device) (Port, error) {
	if d.Driver == nil {
		return nil, fmt.Errorf("usbserial: device has no registered driver")
	}
	return d.Driver.Open(d)
}

// lookupDriver returns the first registered driver whose Matches
// method accepts the given VID/PID, or nil if none match.
func lookupDriver(vid, pid uint16) Driver {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, drv := range drivers {
		if drv.Matches(vid, pid) {
			return drv
		}
	}
	return nil
}
