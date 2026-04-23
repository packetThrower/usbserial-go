// usbserial-open opens the first recognized USB-serial adapter,
// configures the requested baud/framing, and relays stdin↔device.
// Ctrl-] (GS, 0x1D) quits.
//
//	$ go run ./cmd/usbserial-open -baud 57600
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	_ "github.com/packetThrower/usbserial-go/cp210x"
	"github.com/packetThrower/usbserial-go/usbserial"
)

func main() {
	baud := flag.Int("baud", 9600, "baud rate")
	flag.Parse()

	devs, err := usbserial.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list:", err)
		os.Exit(1)
	}
	if len(devs) == 0 {
		fmt.Fprintln(os.Stderr, "no recognized USB-serial adapters attached")
		os.Exit(1)
	}
	d := devs[0]
	fmt.Fprintf(os.Stderr, "opening %s (%04x:%04x)", d.Chipset, d.VendorID, d.ProductID)
	if d.Product != "" {
		fmt.Fprintf(os.Stderr, " — %s", d.Product)
	}
	fmt.Fprintln(os.Stderr)

	p, err := usbserial.Open(d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer p.Close()

	if err := p.SetBaudRate(*baud); err != nil {
		fmt.Fprintln(os.Stderr, "set baud:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "open @ %d-8N1. Ctrl-] to quit.\n", *baud)

	// Device → stdout.
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(os.Stdout, p)
		done <- err
	}()
	// stdin → device, watching for Ctrl-] (0x1D) as the quit byte.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				for i := 0; i < n; i++ {
					if buf[i] == 0x1D {
						done <- nil
						return
					}
				}
				if _, werr := p.Write(buf[:n]); werr != nil {
					done <- werr
					return
				}
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()
	<-done
}
