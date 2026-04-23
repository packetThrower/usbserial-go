// usbserial-list enumerates attached USB-serial adapters that any
// registered chipset driver knows how to handle. Run without
// arguments; prints one line per device.
//
//	$ go run ./cmd/usbserial-list
//	cp210x — 0123456789 at usb:bus=001:addr=004 (10c4:ea60, CP2102 / CP2102N / CP2104)
package main

import (
	"fmt"
	"os"

	"github.com/packetThrower/usbserial-go/usbserial"
	_ "github.com/packetThrower/usbserial-go/cp210x"
	// Add further chipset packages here as they land:
	// _ "github.com/packetThrower/usbserial-go/ftdi"
	// _ "github.com/packetThrower/usbserial-go/ch341"
	// _ "github.com/packetThrower/usbserial-go/pl2303"
)

func main() {
	devs, err := usbserial.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list:", err)
		os.Exit(1)
	}
	if len(devs) == 0 {
		fmt.Fprintln(os.Stderr, "no recognized USB-serial adapters attached")
		return
	}
	for _, d := range devs {
		fmt.Printf("%s — %s at %s (%04x:%04x)\n",
			d.Chipset, d.Serial, d.Path, d.VendorID, d.ProductID)
	}
}
