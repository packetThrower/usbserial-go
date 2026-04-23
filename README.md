# usbserial-go

A Go library for talking to USB-to-serial adapters directly from
userspace, bypassing chipset-specific kernel drivers on Linux and
macOS. Windows falls back to the OS's existing vendor driver via
`go.bug.st/serial` (Windows requires kernel-mode drivers for USB
devices, no reasonable userspace alternative exists without shipping
a signed driver).

The motivation: on Linux and macOS, SiLabs / FTDI / Prolific / WCH
chipsets each ship with their own kernel drivers or kext-era system
extensions that users have to install, authorize, and sometimes
troubleshoot. libusb can talk to any of these chips directly by
implementing their documented USB control-request protocols, which
means an app can open a serial adapter with zero driver install. This
library aims to provide that for the common chipsets, exposed through
a single unified API.

## Status

**Pre-alpha.** Scaffolding only. No chipset protocols implemented yet.

Planned rollout:

- **v0.1.0** — Core `Port` interface, chipset detection / enumeration,
  CP210x (SiLabs) implementation as the proof-of-concept.
- **v0.2.0** — CH341 (WCH) — ubiquitous in cheap USB-serial adapters.
- **v0.3.0** — FTDI (FT232R, FT2232H) — quality adapters and industrial.
- **v0.4.0** — PL2303 (Prolific). Last because of the counterfeit-
  detection social landscape; see [pl2303/README.md](pl2303/README.md).

## Platform support

| Platform | Approach | User prerequisites |
|---|---|---|
| **Linux** | Direct libusb. `detach_kernel_driver()` reclaims the device from the in-kernel `cp210x` / `ftdi_sio` / etc. modules. | User in `plugdev` or `dialout` group (distro-dependent), or udev rules for the target VID/PID. |
| **macOS** | Direct libusb via IOKit. No entitlements required for non-sandboxed apps. | The SiLabs / FTDI kext / system extension must *not* be installed. If already installed, remove it before using this library. |
| **Windows** | Falls through to `go.bug.st/serial` / native COM-port API. | User installs the chipset vendor's Windows driver as usual (SiLabs VCP, FTDI VCP, etc.). |

## Installation

```sh
go get github.com/packetThrower/usbserial-go
```

**System dependencies (Linux and macOS only):** `libusb-1.0` at
runtime, plus `pkg-config` at build time (gousb uses it to locate
libusb-1.0's headers and libs).

- macOS: `brew install pkg-config libusb`
- Debian / Ubuntu: `sudo apt install pkg-config libusb-1.0-0-dev`
- Fedora: `sudo dnf install pkgconf-pkg-config libusb1-devel`
- Arch: `sudo pacman -S pkgconf libusb`

Windows builds skip both entirely — build with `CGO_ENABLED=0`
(falls through to `go.bug.st/serial`, which is pure Go).

## Usage

```go
import (
    "github.com/packetThrower/usbserial-go/usbserial"
    // Blank-import the chipset subpackages you want to register.
    // Only the ones you import are linked into the final binary.
    _ "github.com/packetThrower/usbserial-go/cp210x"
)

func main() {
    devs, err := usbserial.List()
    if err != nil {
        log.Fatal(err)
    }
    for _, d := range devs {
        fmt.Printf("%s — %s @ %s\n", d.Chipset, d.Serial, d.Path)
    }

    port, err := usbserial.Open(devs[0])
    if err != nil {
        log.Fatal(err)
    }
    defer port.Close()

    port.SetBaudRate(115200)
    port.SetFraming(usbserial.Framing{DataBits: 8, StopBits: 1, Parity: usbserial.ParityNone})

    // port implements io.ReadWriter
    fmt.Fprintln(port, "show version")
    buf := make([]byte, 1024)
    n, _ := port.Read(buf)
    fmt.Print(string(buf[:n]))
}
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (TODO).

Chipset additions are welcome. The broad recipe:

1. Read the vendor's USB protocol documentation (datasheet or
   application note).
2. For reference, cross-check against the Linux kernel's USB serial
   drivers: [drivers/usb/serial](https://github.com/torvalds/linux/tree/master/drivers/usb/serial).
3. Implement the [`usbserial.Port`](usbserial/port.go) interface
   inside a new subpackage.
4. Register VID/PID entries in the package's `init()` so
   `usbserial.List()` picks them up automatically.
5. Ship a minimal test against real hardware at common baud rates.

**Licensing note:** implementations should be written from the
vendor's public datasheet, not by translating the Linux kernel's
GPLv2 drivers line-by-line. Clean-room implementations let this
library stay MIT-licensed and consumable by permissive projects.

## License

[MIT](LICENSE). Permissive on purpose — this library is meant to be
useful to anyone writing serial-over-USB software in Go, not just
one downstream app.
