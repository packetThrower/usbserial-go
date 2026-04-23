# usbserial-go

A Go library for USB-to-serial adapters. Opens devices directly
through libusb on Linux and macOS by implementing the chipset's
own USB protocol in Go; falls through to `go.bug.st/serial` on
Windows. One `Port` interface, same code path per platform.

## What it's actually for

USB-to-serial bridges (Silicon Labs CP210x, Prolific PL2303, WCH
CH340/CH341, FTDI FT232/FT2232) each speak a different
vendor-specific USB protocol. The normal path is that the OS's own
driver recognises the device's VID:PID, attaches to it, and exposes
a `/dev/tty*` or `COMx` node. Modern OSes cover most of that well:

- **Linux** ships `cp210x`, `ftdi_sio`, `pl2303`, `ch341`, and
  `cdc_acm` in mainline. Plug in a stock-VID device and a
  `/dev/ttyUSB*` (or `/dev/ttyACM*`) node appears with no user
  action — tested on Ubuntu 24.04 LTS.
- **macOS 11+** ships Apple-bundled DEXTs for all four common
  chipsets: `AppleUSBFTDI`, `AppleUSBPLCOM` (Prolific),
  `AppleUSBSLCOM` (Silicon Labs CP210x), `AppleUSBCHCOM` (WCH
  CH340/CH341), plus `AppleUSBCDCACMData` for CDC-ACM. Each of
  those drivers matches a specific, hard-coded VID:PID table in
  its Info.plist — so a **stock** CP2102 or CH340G or PL2303HXD
  just works, but anything not in the table still needs a driver
  or something like this library.
- **Windows** needs vendor drivers for everything except CDC-ACM,
  but the install flow is well-trodden and the vendor's own
  installer handles it.

This library isn't trying to replace any of that. It's for the
cases that fall through the cracks:

1. **Vendor-rebranded VIDs on any OS** — a common chipset reflashed
   with some vendor's own USB-IF VID so the OS driver's id_table
   doesn't match. Example: Siemens's RUGGEDCOM USB Serial console
   is a Silicon Labs CP210x burned with Siemens VID
   `0x0908:0x01FF`. Neither `AppleUSBSLCOM` nor Linux's `cp210x`
   list that pair, so no `/dev/cu.*` / `/dev/ttyUSB*` appears
   (on Linux, a manual
   `echo 0908 01ff | sudo tee /sys/bus/usb-serial/drivers/cp210x/new_id`
   works; on macOS, nothing short of a third-party driver did
   before this library). The same pattern shows up across a lot
   of industrial and instrument gear.
2. **Less-common PIDs under a stock VID** that the OS id_table
   happens to miss — CP2108 quad-UART (`10c4:ea71`) is a common
   example; `AppleUSBSLCOM` lists only the CP2102/CP2105 pairs.
3. **Legacy PL2303HXA cables on macOS** that Apple's
   `AppleUSBPLCOM` deliberately excludes (the "counterfeit"
   situation — Apple and Prolific's current driver both refuse to
   bind to the oldest chip revisions).

On **older macOS versions** (before the Apple-bundled DEXTs
existed), this library also covers the stock-VID case — but the
intended user base is all on macOS 11+, so that's a side effect
rather than the motivation.

On **Linux**, vendor rebrands and atypical PIDs are also the
primary gap — the kernel modules themselves are fine for stock
hardware.

For those cases, usbserial-go opens the device via libusb, runs
the chipset's USB control-request protocol (baud, framing, flow,
DTR/RTS, break, modem status), and shuttles bytes over the bulk
IN/OUT endpoints. No driver install, no sysfs dance, no kext
approval.

## Status

Implemented and tested against real hardware:

- **CP210x (Silicon Labs)** — open, read, write, baud, framing,
  flow control (none / RTSCTS / XONXOFF), DTR/RTS, modem status,
  break. Covers CP2102, CP2102N, CP2104, plus the Siemens RUGGEDCOM
  rebrand. Multi-UART variants (CP2105 dual, CP2108 quad) enumerate
  fine but currently only expose the first UART.

Planned:

- **CH340/CH341 (WCH)** — next up. `AppleUSBCHCOM` covers only
  two PIDs (`1a86:7523` and `1a86:55d4`); anything else under the
  WCH VID — or any WCH chip burned under another vendor's VID —
  falls through to this library.
- **FTDI** — mostly for API parity. `AppleUSBFTDI` has a 97-entry
  id_table that covers most FTDI devices and rebrands, so the
  motivation here is uniformity rather than a meaningful gap.
- **PL2303 (Prolific)** — also for parity. `AppleUSBPLCOM` covers
  the common PL2303 chip revisions but deliberately excludes the
  legacy HXA parts, which are the only remaining real macOS gap
  for Prolific. See [pl2303/README.md](pl2303/README.md).

## Platform support

| Platform | Transport | Notes |
|---|---|---|
| **Linux** | libusb (via gousb). `SetAutoDetach(true)` unbinds the in-kernel `cp210x` / `ftdi_sio` / etc. module on claim and re-binds on release. | Requires libusb access for the user — udev rule for the target VID/PID, or membership in `plugdev`/`dialout`. |
| **macOS** | libusb (via gousb). `SetAutoDetach` is skipped here. | No entitlements needed for devices whose VID:PID isn't claimed by any Apple DEXT (vendor rebrands, less-common PIDs). Devices already claimed by `AppleUSBSLCOM` / `AppleUSBCHCOM` / `AppleUSBPLCOM` / `AppleUSBFTDI` can't be detached without a DriverKit entitlement — for those, Apple's driver is doing the right thing and you should use it via the normal `/dev/cu.*` path. |
| **Windows** | `go.bug.st/serial`. | Vendor driver installed as usual. This library is a thin API wrapper on Windows. |

## Installation

```sh
go get github.com/packetThrower/usbserial-go
```

Linux and macOS builds need `libusb-1.0` at runtime and `pkg-config`
at build time (gousb uses it to find libusb's headers and libs):

- macOS: `brew install pkg-config libusb`
- Debian / Ubuntu: `sudo apt install pkg-config libusb-1.0-0-dev`
- Fedora: `sudo dnf install pkgconf-pkg-config libusb1-devel`
- Arch: `sudo pacman -S pkgconf libusb`

Windows builds skip both entirely — build with `CGO_ENABLED=0`
(the Windows build path is pure Go via `go.bug.st/serial`).

## Usage

```go
import (
    _ "github.com/packetThrower/usbserial-go/cp210x"
    "github.com/packetThrower/usbserial-go/usbserial"
)

func main() {
    devs, err := usbserial.List()
    if err != nil {
        log.Fatal(err)
    }
    for _, d := range devs {
        fmt.Printf("%s %04x:%04x — %s / %s / serial=%s / %s\n",
            d.Chipset, d.VendorID, d.ProductID,
            d.Manufacturer, d.Product, d.Serial, d.Path)
    }

    port, err := usbserial.Open(devs[0])
    if err != nil {
        log.Fatal(err)
    }
    defer port.Close()

    port.SetBaudRate(115200)
    port.SetFraming(usbserial.Framing{
        DataBits: 8,
        StopBits: 1,
        Parity:   usbserial.ParityNone,
    })

    // port implements io.ReadWriteCloser.
    fmt.Fprintln(port, "show version")
    buf := make([]byte, 1024)
    n, _ := port.Read(buf)
    fmt.Print(string(buf[:n]))
}
```

Blank-importing a chipset subpackage is how the library's link-time
registry learns which chipsets to enumerate. Only the subpackages
you import contribute code to the final binary.

### CLIs in the repo

Two small tools useful for testing an install:

- `go run ./cmd/usbserial-list` — lists every attached device that a
  registered chipset subpackage claims, with full descriptor info.
- `go run ./cmd/usbserial-open -baud 115200` — opens the first such
  device, pipes stdin → device-TX and device-RX → stdout. Ctrl-] quits.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (TODO).

Chipset additions are welcome. The recipe:

1. Read the vendor's USB protocol documentation — datasheet or
   application note. SiLabs AN571 is the reference for CP210x;
   equivalents exist for each other chipset.
2. Cross-check against the Linux kernel's USB serial drivers at
   [drivers/usb/serial](https://github.com/torvalds/linux/tree/master/drivers/usb/serial)
   for tricky details (quirk lists, oddly-behaved chips).
3. Implement the [`usbserial.Port`](usbserial/port.go) interface in
   a new subpackage. Use the `cp210x/` subpackage as a reference
   layout: `ids.go` registers the driver, `port_unix.go` is the
   libusb implementation, `port_windows.go` is the
   `go.bug.st/serial` passthrough.
4. Register VID/PID entries — and any vendor-rebrand entries — so
   `usbserial.List()` picks up the device automatically.
5. Ship a test against real hardware at 9600 and 115200 baud.

Implementations should be written from the vendor's public
datasheet, not by translating the Linux kernel's GPLv2 drivers
line by line. Clean-room implementations keep this library
MIT-licensed and consumable by permissive projects.

## License

[MIT](LICENSE). Permissive on purpose — this library is meant to be
useful to anyone writing serial-over-USB software in Go, not just
one downstream app.
