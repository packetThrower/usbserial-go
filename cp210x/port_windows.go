//go:build windows

package cp210x

import (
	"fmt"
	"sync"
	"time"

	bsserial "go.bug.st/serial"
	"github.com/packetThrower/usbserial-go/usbserial"
)

// port is the Windows implementation of usbserial.Port for CP210x.
// Windows has no practical userspace path to a CP210x without
// shipping a signed kernel driver, so we let the OS's vendor driver
// (SiLabs VCP) do the heavy lifting and drive it through the normal
// COM-port API via go.bug.st/serial. The user still gets the same
// Port interface; it's just backed by a different transport.
type port struct {
	p bsserial.Port

	// mu serialises SetBaudRate / SetFraming / SetFlowControl
	// because each of them rewrites the full serial.Mode — reading
	// the current mode and writing it back isn't atomic on the
	// underlying driver's side, so two concurrent setters could
	// race and lose one another's change.
	mu   sync.Mutex
	mode bsserial.Mode
}

// openPort opens the COM port named in d.Path and returns a Port
// with 9600-8N1 defaults that the caller will usually override
// before traffic. Matches the Linux/macOS bring-up semantics so
// consumers don't have to branch on platform.
func openPort(target usbserial.Device) (usbserial.Port, error) {
	if target.Path == "" {
		return nil, fmt.Errorf("cp210x: no COM port in device path")
	}
	mode := bsserial.Mode{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   bsserial.NoParity,
		StopBits: bsserial.OneStopBit,
	}
	bsp, err := bsserial.Open(target.Path, &mode)
	if err != nil {
		return nil, fmt.Errorf("cp210x: open %s: %w", target.Path, err)
	}
	return &port{p: bsp, mode: mode}, nil
}

func (p *port) Read(buf []byte) (int, error)  { return p.p.Read(buf) }
func (p *port) Write(buf []byte) (int, error) { return p.p.Write(buf) }
func (p *port) Close() error                  { return p.p.Close() }

func (p *port) SetBaudRate(baud int) error {
	if baud <= 0 {
		return fmt.Errorf("cp210x: baud rate must be positive, got %d", baud)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode.BaudRate = baud
	return p.p.SetMode(&p.mode)
}

func (p *port) SetFraming(f usbserial.Framing) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if f.DataBits < 5 || f.DataBits > 8 {
		return fmt.Errorf("cp210x: data bits must be 5..8, got %d", f.DataBits)
	}
	p.mode.DataBits = f.DataBits

	switch f.StopBits {
	case 1:
		p.mode.StopBits = bsserial.OneStopBit
	case 15:
		p.mode.StopBits = bsserial.OnePointFiveStopBits
	case 2:
		p.mode.StopBits = bsserial.TwoStopBits
	default:
		return fmt.Errorf("cp210x: stop bits must be 1, 15 (=1.5), or 2; got %d", f.StopBits)
	}

	switch f.Parity {
	case usbserial.ParityNone:
		p.mode.Parity = bsserial.NoParity
	case usbserial.ParityOdd:
		p.mode.Parity = bsserial.OddParity
	case usbserial.ParityEven:
		p.mode.Parity = bsserial.EvenParity
	case usbserial.ParityMark:
		p.mode.Parity = bsserial.MarkParity
	case usbserial.ParitySpace:
		p.mode.Parity = bsserial.SpaceParity
	default:
		return fmt.Errorf("cp210x: unknown parity %d", f.Parity)
	}
	return p.p.SetMode(&p.mode)
}

func (p *port) SetFlowControl(fc usbserial.FlowControl) error {
	// go.bug.st/serial doesn't expose flow control on its Mode
	// struct; on Windows there's no reliable portable knob for it
	// anyway (the SiLabs driver's flow control is configured in
	// Device Manager and can't be overridden from user-space
	// without driver-specific IOCTLs). Treat this as a no-op for
	// FlowNone and report an error for the others so consumers
	// can surface it.
	if fc == usbserial.FlowNone {
		return nil
	}
	return fmt.Errorf("cp210x: flow control beyond FlowNone is not wired through go.bug.st/serial on Windows")
}

func (p *port) SetDTR(assert bool) error { return p.p.SetDTR(assert) }
func (p *port) SetRTS(assert bool) error { return p.p.SetRTS(assert) }

func (p *port) GetModemStatus() (usbserial.ModemStatus, error) {
	bits, err := p.p.GetModemStatusBits()
	if err != nil {
		return usbserial.ModemStatus{}, fmt.Errorf("cp210x: get modem status: %w", err)
	}
	return usbserial.ModemStatus{
		CTS: bits.CTS,
		DSR: bits.DSR,
		RI:  bits.RI,
		DCD: bits.DCD,
	}, nil
}

func (p *port) SendBreak(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("cp210x: break duration must be positive, got %s", d)
	}
	return p.p.Break(d)
}
