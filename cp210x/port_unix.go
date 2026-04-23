//go:build linux || darwin

package cp210x

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gousb"
	"github.com/packetThrower/usbserial-go/usbserial"
)

// port is the libusb-backed CP210x implementation of usbserial.Port
// for Linux and macOS. Everything below the Port surface is in terms
// of libusb control transfers (AN571) and bulk endpoint reads/writes.
//
// Lifecycle ownership:
//
//	ctx    - owned, closed in Close
//	dev    - owned, closed in Close
//	cfg    - owned, closed in Close
//	intf   - owned, released in Close
//	inEp/outEp - owned by intf; released when intf is closed
//
// Cancellation:
//
//	rwCtx is cancelled in Close so a blocked Read or Write returns
//	promptly. libusb's own cancel-transfer is invoked under the hood
//	by gousb's ReadContext/WriteContext.
type port struct {
	ctx   *gousb.Context
	dev   *gousb.Device
	cfg   *gousb.Config
	intf  *gousb.Interface
	inEp  *gousb.InEndpoint
	outEp *gousb.OutEndpoint

	// iface is the USB interface number we issue control requests
	// against (wIndex in the setup packet). Hardcoded to 0 today;
	// CP2105/CP2108 multi-UART variants will need this to become
	// per-port state when we add support.
	iface uint16

	// rwCtx drives cancellation of in-flight Read/Write. Replaced
	// with context.Background and cancelled once in Close.
	rwCtx    context.Context
	cancelRW context.CancelFunc

	// ctrlMu serialises control transfers among the SetXxx methods
	// and Close. The Port contract doesn't require control methods
	// to be concurrent-safe, but Close is called from a different
	// goroutine than SetBaudRate in realistic consumers, and having
	// Close tear down the endpoints mid-transfer would be a sharp
	// edge. This keeps the surface predictable.
	ctrlMu sync.Mutex

	// dtr/rts track the last-written state of the handshake lines
	// so each setter can toggle one without clobbering the other
	// (SET_MHS updates both, gated by the per-line mask bits).
	dtr atomic.Bool
	rts atomic.Bool

	closed atomic.Bool
}

// openPort locates the USB device described by d, opens it via
// libusb, claims interface 0, and runs the CP210x bring-up sequence
// (IFC_ENABLE=1, then sensible 9600-8N1 defaults). Matching uses
// VID:PID first and narrows by Serial when one was recorded — that
// way the port survives a re-plug between List and Open without
// relying on bus/address stability.
func openPort(target usbserial.Device) (usbserial.Port, error) {
	usbCtx := gousb.NewContext()

	dev, err := findDevice(usbCtx, target)
	if err != nil {
		usbCtx.Close()
		return nil, err
	}

	// On Linux, if the cp210x kernel module is installed it'll have
	// bound to this device already — SetAutoDetach tells libusb to
	// unbind it when we claim, and re-bind when we release.
	//
	// On macOS the call itself can trip on devices matched by
	// AppleUSBHostCompositeDevice (the generic shim that attaches
	// to any composite device without a specific driver); libusb
	// tries to detach and the IOKit capability check rejects it
	// with LIBUSB_ERROR_ACCESS because no DriverKit entitlement is
	// set on the process. Skip the auto-detach there — we only
	// actually need it on Linux where a real vendor kernel module
	// might be fighting us for the interface.
	if runtime.GOOS == "linux" {
		if err := dev.SetAutoDetach(true); err != nil {
			// Not fatal — the claim below will fail cleanly if
			// something's still bound that shouldn't be.
		}
	}

	// Config 1 is the only configuration on every CP210x variant.
	// Config() claims it and must be Close()d later.
	cfg, err := dev.Config(1)
	if err != nil {
		dev.Close()
		usbCtx.Close()
		return nil, fmt.Errorf("cp210x: select config 1: %w", err)
	}

	// Interface 0 alt 0. Multi-UART variants expose additional
	// interfaces (CP2105 has 0 + 1, CP2108 has 0..3) but we only
	// support the first one today.
	intf, err := cfg.Interface(0, 0)
	if err != nil {
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return nil, fmt.Errorf("cp210x: claim interface 0: %w", err)
	}

	inNum, outNum, err := findBulkEndpoints(intf.Setting)
	if err != nil {
		intf.Close()
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return nil, err
	}
	inEp, err := intf.InEndpoint(inNum)
	if err != nil {
		intf.Close()
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return nil, fmt.Errorf("cp210x: open bulk IN endpoint %d: %w", inNum, err)
	}
	outEp, err := intf.OutEndpoint(outNum)
	if err != nil {
		intf.Close()
		cfg.Close()
		dev.Close()
		usbCtx.Close()
		return nil, fmt.Errorf("cp210x: open bulk OUT endpoint %d: %w", outNum, err)
	}

	p := &port{
		ctx:   usbCtx,
		dev:   dev,
		cfg:   cfg,
		intf:  intf,
		inEp:  inEp,
		outEp: outEp,
		iface: 0,
	}
	p.rwCtx, p.cancelRW = context.WithCancel(context.Background())

	// Bring-up sequence. Anything that fails here tears the port
	// back down cleanly — we don't want to leak an enabled UART on
	// a failed open.
	if err := p.setIfcEnable(true); err != nil {
		p.teardown()
		return nil, err
	}
	// Default to 9600-8N1, no flow. The caller almost always
	// overrides before first traffic, but this avoids a silent
	// "you need to call the setters first" failure mode if they
	// don't — 9600-8N1 is the universal legacy console default.
	if err := p.SetBaudRate(9600); err != nil {
		p.teardown()
		return nil, err
	}
	if err := p.SetFraming(usbserial.Framing{DataBits: 8, StopBits: 1, Parity: usbserial.ParityNone}); err != nil {
		p.teardown()
		return nil, err
	}
	if err := p.SetFlowControl(usbserial.FlowNone); err != nil {
		p.teardown()
		return nil, err
	}
	// DTR/RTS asserted by default — matches go.bug.st/serial's
	// behaviour on open and is what Cisco/Juniper consoles expect
	// to see on a healthy session.
	p.dtr.Store(true)
	p.rts.Store(true)
	if err := p.writeMhs(true, true, true, true); err != nil {
		p.teardown()
		return nil, err
	}

	return p, nil
}

// findDevice re-enumerates the bus inside a fresh libusb context
// and returns a handle to the device whose (VID, PID, Serial) match
// target. Any extra matches (duplicate adapters) are closed. Called
// during Open — the Device returned by List was closed along with
// its parent context, so we can't reuse those handles here.
func findDevice(ctx *gousb.Context, target usbserial.Device) (*gousb.Device, error) {
	matches, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return uint16(desc.Vendor) == target.VendorID && uint16(desc.Product) == target.ProductID
	})
	if err != nil && len(matches) == 0 {
		return nil, fmt.Errorf("cp210x: enumerate: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("cp210x: device %04x:%04x not found", target.VendorID, target.ProductID)
	}

	if target.Serial != "" {
		for i, m := range matches {
			raw, _ := m.SerialNumber()
			if usbserial.TrimDescriptor(raw) == target.Serial {
				closeAllExcept(matches, i)
				return m, nil
			}
		}
		closeAllExcept(matches, -1)
		return nil, fmt.Errorf("cp210x: device %04x:%04x with serial %q not found",
			target.VendorID, target.ProductID, target.Serial)
	}

	closeAllExcept(matches, 0)
	return matches[0], nil
}

// closeAllExcept closes every gousb.Device in matches except the
// one at keepIdx (pass -1 to close them all).
func closeAllExcept(matches []*gousb.Device, keepIdx int) {
	for i, m := range matches {
		if i == keepIdx {
			continue
		}
		_ = m.Close()
	}
}

// findBulkEndpoints locates the first bulk IN and bulk OUT endpoint
// numbers on an interface setting. CP2102 consistently uses number
// 1 for both (addresses 0x01 OUT / 0x81 IN), but scanning keeps us
// honest for variants that don't.
func findBulkEndpoints(setting gousb.InterfaceSetting) (inNum, outNum int, err error) {
	inNum, outNum = -1, -1
	for _, ep := range setting.Endpoints {
		if ep.TransferType != gousb.TransferTypeBulk {
			continue
		}
		switch ep.Direction {
		case gousb.EndpointDirectionIn:
			if inNum == -1 {
				inNum = ep.Number
			}
		case gousb.EndpointDirectionOut:
			if outNum == -1 {
				outNum = ep.Number
			}
		}
	}
	if inNum == -1 || outNum == -1 {
		return 0, 0, errors.New("cp210x: no bulk IN/OUT endpoint pair on interface 0")
	}
	return inNum, outNum, nil
}

// --- Port interface: I/O --------------------------------------------------

func (p *port) Read(buf []byte) (int, error) {
	if p.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	n, err := p.inEp.ReadContext(p.rwCtx, buf)
	if err != nil && p.closed.Load() {
		// Racing with Close; report that cleanly.
		return n, io.ErrClosedPipe
	}
	return n, err
}

func (p *port) Write(buf []byte) (int, error) {
	if p.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	n, err := p.outEp.WriteContext(p.rwCtx, buf)
	if err != nil && p.closed.Load() {
		return n, io.ErrClosedPipe
	}
	return n, err
}

func (p *port) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Unblock anything waiting on Read/Write first so teardown
	// doesn't race with an in-flight transfer.
	p.cancelRW()

	// Best-effort UART disable. If the device is already gone
	// (physical unplug) the control transfer fails; we still
	// want to free our libusb handles regardless.
	p.ctrlMu.Lock()
	_ = p.controlOut(reqIfcEnable, ifcDisable, nil)
	p.ctrlMu.Unlock()

	p.teardown()
	return nil
}

// teardown releases the libusb handles in the opposite order they
// were acquired. Safe to call multiple times; nil-safe for partial
// construction during a failed Open.
func (p *port) teardown() {
	if p.intf != nil {
		p.intf.Close()
		p.intf = nil
	}
	if p.cfg != nil {
		_ = p.cfg.Close()
		p.cfg = nil
	}
	if p.dev != nil {
		_ = p.dev.Close()
		p.dev = nil
	}
	if p.ctx != nil {
		_ = p.ctx.Close()
		p.ctx = nil
	}
}

// --- Port interface: settings ---------------------------------------------

func (p *port) SetBaudRate(baud int) error {
	if baud <= 0 {
		return fmt.Errorf("cp210x: baud rate must be positive, got %d", baud)
	}
	// SET_BAUDRATE carries the rate as a little-endian uint32 in
	// the data stage — unlike the pack-into-wValue setters below.
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, uint32(baud))
	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqSetBaudRate, 0, payload)
}

func (p *port) SetFraming(f usbserial.Framing) error {
	if f.DataBits < 5 || f.DataBits > 8 {
		return fmt.Errorf("cp210x: data bits must be 5..8, got %d", f.DataBits)
	}

	// Stop bits: 1 -> 0, 1.5 -> 1 (encoded as 15 in the Port API),
	// 2 -> 2. The oddball 1.5 value is only valid at 5 data bits on
	// most chipsets but we defer that policy to the caller.
	var stopCode uint16
	switch f.StopBits {
	case 1:
		stopCode = 0
	case 15:
		stopCode = 1
	case 2:
		stopCode = 2
	default:
		return fmt.Errorf("cp210x: stop bits must be 1, 15 (=1.5), or 2; got %d", f.StopBits)
	}

	var parityCode uint16
	switch f.Parity {
	case usbserial.ParityNone:
		parityCode = 0
	case usbserial.ParityOdd:
		parityCode = 1
	case usbserial.ParityEven:
		parityCode = 2
	case usbserial.ParityMark:
		parityCode = 3
	case usbserial.ParitySpace:
		parityCode = 4
	default:
		return fmt.Errorf("cp210x: unknown parity %d", f.Parity)
	}

	// See SET_LINE_CTL bit layout in protocol.go.
	value := (uint16(f.DataBits) << 8) | (parityCode << 4) | stopCode
	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqSetLineCtl, value, nil)
}

func (p *port) SetFlowControl(fc usbserial.FlowControl) error {
	// AN571 §5.17: SET_FLOW takes a 16-byte payload of four u32
	// fields in LE order (ControlHandshake, FlowReplace, XonLimit,
	// XoffLimit). The bit semantics are dense and easy to get
	// wrong; the three presets below match what the Linux kernel's
	// cp210x.c sends for the same three modes, which is the most
	// field-tested implementation anywhere.
	var ctrl, replace uint32
	switch fc {
	case usbserial.FlowNone:
		// DTR held under host control (bit 0 = 1, bits 1 = 0);
		// CTS handshake off. RTS also under host control
		// (FlowReplace bits 6-7 = 01 = RTS_CONTROL_ENABLE).
		ctrl = 0x00000001
		replace = 0x00000040
	case usbserial.FlowRTSCTS:
		// DTR host-controlled + CTS-handshake on (bit 3).
		// RTS driven as flow-control line (bits 6-7 = 10).
		ctrl = 0x00000009
		replace = 0x00000080
	case usbserial.FlowXONXOFF:
		// DTR host-controlled; TX+RX software flow on (bits 7 + 8);
		// RTS left under host control.
		ctrl = 0x00000181
		replace = 0x00000040
	default:
		return fmt.Errorf("cp210x: unknown flow control %d", fc)
	}

	payload := make([]byte, 16)
	binary.LittleEndian.PutUint32(payload[0:4], ctrl)
	binary.LittleEndian.PutUint32(payload[4:8], replace)
	binary.LittleEndian.PutUint32(payload[8:12], 0x80) // XonLimit: 128 bytes remaining
	binary.LittleEndian.PutUint32(payload[12:16], 0x80) // XoffLimit: 128 bytes queued

	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqSetFlow, 0, payload)
}

func (p *port) SetDTR(assert bool) error {
	p.dtr.Store(assert)
	return p.writeMhs(assert, p.rts.Load(), true, false)
}

func (p *port) SetRTS(assert bool) error {
	p.rts.Store(assert)
	return p.writeMhs(p.dtr.Load(), assert, false, true)
}

// writeMhs issues a SET_MHS with the per-line mask bits selecting
// which of DTR/RTS the chip should apply. Callers that only want
// to change one line pass false for the other line's mask bit;
// the inactive mask bit makes the chip ignore the corresponding
// value bit, so we can pass the current state unconditionally.
func (p *port) writeMhs(dtr, rts, dtrMask, rtsMask bool) error {
	var value uint16
	if dtr {
		value |= mhsDTR
	}
	if rts {
		value |= mhsRTS
	}
	if dtrMask {
		value |= mhsDTRMask
	}
	if rtsMask {
		value |= mhsRTSMask
	}
	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqSetMhs, value, nil)
}

func (p *port) GetModemStatus() (usbserial.ModemStatus, error) {
	payload := make([]byte, 1)
	p.ctrlMu.Lock()
	err := p.controlIn(reqGetMdmSts, 0, payload)
	p.ctrlMu.Unlock()
	if err != nil {
		return usbserial.ModemStatus{}, err
	}
	b := payload[0]
	return usbserial.ModemStatus{
		CTS: b&mdmCTS != 0,
		DSR: b&mdmDSR != 0,
		RI:  b&mdmRI != 0,
		DCD: b&mdmDCD != 0,
	}, nil
}

func (p *port) SendBreak(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("cp210x: break duration must be positive, got %s", d)
	}
	p.ctrlMu.Lock()
	if err := p.controlOut(reqSetBreak, 1, nil); err != nil {
		p.ctrlMu.Unlock()
		return err
	}
	p.ctrlMu.Unlock()

	// Hold the break condition without the control-mutex held so
	// other setters can proceed (they're not generally useful mid-
	// break, but we don't want to serialise them needlessly).
	time.Sleep(d)

	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqSetBreak, 0, nil)
}

// setIfcEnable toggles the UART interface on or off via IFC_ENABLE.
// Called during Open (enable) and Close (disable).
func (p *port) setIfcEnable(enable bool) error {
	val := ifcDisable
	if enable {
		val = ifcEnable
	}
	p.ctrlMu.Lock()
	defer p.ctrlMu.Unlock()
	return p.controlOut(reqIfcEnable, val, nil)
}

// controlOut sends a host→device vendor control request to
// interface p.iface. The caller holds p.ctrlMu.
func (p *port) controlOut(req uint8, value uint16, data []byte) error {
	// bmRequestType = 0x41: host→device (bit 7 = 0), vendor (bits
	// 6:5 = 01), interface recipient (bits 4:0 = 00001).
	n, err := p.dev.Control(0x41, req, value, p.iface, data)
	if err != nil {
		return fmt.Errorf("cp210x: control OUT req=%02x: %w", req, err)
	}
	if n != len(data) {
		return fmt.Errorf("cp210x: control OUT req=%02x: short transfer %d/%d", req, n, len(data))
	}
	return nil
}

// controlIn issues a device→host vendor control request and fills
// data with the response. The caller holds p.ctrlMu.
func (p *port) controlIn(req uint8, value uint16, data []byte) error {
	// bmRequestType = 0xC1: device→host (bit 7 = 1), vendor,
	// interface recipient.
	n, err := p.dev.Control(0xC1, req, value, p.iface, data)
	if err != nil {
		return fmt.Errorf("cp210x: control IN req=%02x: %w", req, err)
	}
	if n != len(data) {
		return fmt.Errorf("cp210x: control IN req=%02x: short transfer %d/%d", req, n, len(data))
	}
	return nil
}
