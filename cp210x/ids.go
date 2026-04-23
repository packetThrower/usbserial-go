// Package cp210x implements the SiLabs CP210x USB-to-UART bridge
// protocol for Linux and macOS. Windows builds of this package are a
// no-op; consumers should fall through to the OS vendor driver via
// go.bug.st/serial instead.
//
// Reference: SiLabs AN571 "CP210x/CP211x USB to UART Bridge VCP
// Interface Specification".
//
//	https://www.silabs.com/documents/public/application-notes/AN571.pdf
package cp210x

import "github.com/packetThrower/usbserial-go/usbserial"

// SiLabs vendor ID. All CP210x-family chips share this VID when they
// ship under the SiLabs USB-IF allocation.
const VendorID = 0x10C4

// Known product IDs across the CP210x family. Not exhaustive — the
// chips have been OEM'd under many PIDs; add entries as hardware
// shows up in the wild.
//
// Source: linux/drivers/usb/serial/cp210x.c id_table[] and vendor
// application note AN571.
var productIDs = map[uint16]string{
	0xEA60: "CP2102 / CP2102N / CP2104",
	0xEA70: "CP2105", // Dual UART
	0xEA71: "CP2108", // Quad UART
	0xEA80: "CP2110", // HID-to-UART variant
}

// rebrands: (VID<<16 | PID) for CP210x-based devices that ship under
// a non-SiLabs USB-IF VID. The chip protocol is still stock CP210x;
// only the device descriptors are reflashed. Kept separate from
// productIDs so the standard SiLabs lookup stays fast and explicit.
//
// Entries:
//   - 0x0908:0x01FF — Siemens RUGGEDCOM USB Serial console (RST2228
//     and similar). Confirmed CP210x by the device's "USB Vendor
//     Name" = "Silicon Labs" descriptor.
var rebrands = map[uint32]string{
	(0x0908 << 16) | 0x01FF: "CP210x (Siemens RUGGEDCOM)",
}

// Matches implements usbserial.Driver.
type driver struct{}

func (driver) Name() usbserial.Chipset { return usbserial.ChipsetCP210x }

func (driver) Matches(vid, pid uint16) bool {
	if vid == VendorID {
		_, ok := productIDs[pid]
		return ok
	}
	_, ok := rebrands[(uint32(vid)<<16)|uint32(pid)]
	return ok
}

func (driver) Open(d usbserial.Device) (usbserial.Port, error) {
	// TODO: libusb-backed open on Linux/macOS; Windows falls through
	// to go.bug.st/serial via a separate build-tagged file.
	return nil, errUnimplemented
}

func init() {
	usbserial.Register(driver{})
}
