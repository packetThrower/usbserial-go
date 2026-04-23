// usbserial-list enumerates attached USB-serial adapters that any
// registered chipset driver knows how to handle. Run without
// arguments; prints one block per device.
package main

import (
	"fmt"
	"os"

	_ "github.com/packetThrower/usbserial-go/cp210x"
	"github.com/packetThrower/usbserial-go/usbserial"
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
	for i, d := range devs {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s (%04x:%04x)\n", d.Chipset, d.VendorID, d.ProductID)
		if d.Manufacturer != "" {
			fmt.Printf("  manufacturer: %s\n", d.Manufacturer)
		}
		if d.Product != "" {
			fmt.Printf("  product:      %s\n", d.Product)
		}
		if d.Serial != "" {
			fmt.Printf("  serial:       %s\n", d.Serial)
		}
		fmt.Printf("  path:         %s\n", d.Path)
	}
}
