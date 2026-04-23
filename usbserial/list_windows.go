//go:build windows

package usbserial

import (
	"strconv"

	"go.bug.st/serial/enumerator"
)

// platformList enumerates Windows COM ports via go.bug.st/serial's
// detailed-ports enumerator (which surfaces USB VID/PID for ports
// backed by a USB adapter) and returns one Device entry per port
// whose VID/PID is claimed by a registered chipset driver.
func platformList() ([]Device, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}

	var results []Device
	for _, p := range ports {
		if !p.IsUSB {
			continue
		}
		vid, err1 := strconv.ParseUint(p.VID, 16, 16)
		pid, err2 := strconv.ParseUint(p.PID, 16, 16)
		if err1 != nil || err2 != nil {
			continue
		}
		drv := lookupDriver(uint16(vid), uint16(pid))
		if drv == nil {
			continue
		}
		results = append(results, Device{
			Chipset:   drv.Name(),
			VendorID:  uint16(vid),
			ProductID: uint16(pid),
			Serial:    p.SerialNumber,
			Path:      p.Name, // e.g. "COM3"
			Driver:    drv,
		})
	}
	return results, nil
}
