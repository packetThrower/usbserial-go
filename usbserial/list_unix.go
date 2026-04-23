//go:build linux || darwin

package usbserial

import (
	"fmt"

	"github.com/google/gousb"
)

// platformList enumerates USB devices via libusb (through gousb) and
// returns one Device entry per device whose VID/PID is claimed by a
// registered chipset driver. Devices are opened briefly to read the
// iSerial descriptor; the handles are closed before the function
// returns. Actual USB I/O for a selected device happens later via
// the chipset driver's Open().
func platformList() ([]Device, error) {
	ctx := gousb.NewContext()
	defer ctx.Close()

	// OpenDevices walks every attached USB device and calls the
	// matcher with its descriptor. Returning true opens the device
	// (so we can read the serial-number string descriptor); returning
	// false skips it without opening. We do a two-step so devices
	// without a registered driver never get opened.
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return lookupDriver(uint16(desc.Vendor), uint16(desc.Product)) != nil
	})
	// OpenDevices returns a partial list plus a joined error when
	// some devices opened and others failed. We don't abort on err
	// here — still return whatever succeeded, and surface the error
	// for the caller to log if they care.
	defer func() {
		for _, d := range devs {
			_ = d.Close()
		}
	}()

	var results []Device
	for _, d := range devs {
		desc := d.Desc
		drv := lookupDriver(uint16(desc.Vendor), uint16(desc.Product))
		if drv == nil {
			continue // can't happen given the filter above, but belt-and-braces
		}
		serial, _ := d.SerialNumber() // empty string is fine; some chipsets omit iSerial
		results = append(results, Device{
			Chipset:   drv.Name(),
			VendorID:  uint16(desc.Vendor),
			ProductID: uint16(desc.Product),
			Serial:    serial,
			Path:      fmt.Sprintf("usb:bus=%03d:addr=%03d", desc.Bus, desc.Address),
			Driver:    drv,
		})
	}
	return results, err
}
