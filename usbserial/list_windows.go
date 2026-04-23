//go:build windows

package usbserial

// platformList enumerates COM ports on Windows. Stub for now — the
// real implementation will use go.bug.st/serial's enumerator, then
// match each enumerated port's USB VID/PID (if available via the
// device's hardware ID) against registered drivers.
func platformList() ([]Device, error) {
	// TODO: wire up the Windows COM-port enumerator.
	return nil, nil
}
