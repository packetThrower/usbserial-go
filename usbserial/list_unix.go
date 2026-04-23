//go:build linux || darwin

package usbserial

// platformList enumerates USB devices on Linux / macOS. Stub for
// now — the real implementation will use libusb (via gousb or a
// similar wrapper) to walk the bus, extract VID/PID + iSerial, and
// match each against the registered drivers.
func platformList() ([]Device, error) {
	// TODO: wire up libusb enumeration.
	return nil, nil
}
