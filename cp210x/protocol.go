package cp210x

// Vendor-specific USB control request codes. All exchanged via the
// standard USB control endpoint (bmRequestType 0x41 for host→device,
// 0xC1 for device→host). Taken verbatim from SiLabs AN571 §4.
const (
	reqIfcEnable   = 0x00 // Enable / disable the UART interface.
	reqSetBaudDiv  = 0x01 // (deprecated, CP2101 only) Set baud rate by clock divisor.
	reqGetBaudDiv  = 0x02 // (deprecated, CP2101 only) Get baud rate divisor.
	reqSetLineCtl  = 0x03 // Set data bits, stop bits, parity.
	reqGetLineCtl  = 0x04 // Get current data bits / stop bits / parity.
	reqSetBreak    = 0x05 // Drive TX low (break condition).
	reqImmChar     = 0x06 // Send a single character immediately, bypassing the TX FIFO.
	reqSetMhs      = 0x07 // Set DTR / RTS (modem handshake signals).
	reqGetMdmSts   = 0x08 // Get modem status (CTS / DSR / RI / DCD + DTR / RTS readback).
	reqSetXon      = 0x09 // Set software flow XON byte.
	reqSetXoff     = 0x0A // Set software flow XOFF byte.
	reqSetEventMask = 0x0B
	reqGetEventMask = 0x0C
	reqSetChar      = 0x0D
	reqGetChars     = 0x0E
	reqGetProps     = 0x0F
	reqGetCommStatus = 0x10
	reqReset         = 0x11
	reqPurge         = 0x12
	reqSetFlow       = 0x13 // Flow-control mode (none / RTSCTS / XONXOFF).
	reqGetFlow       = 0x14
	reqEmbedEvents   = 0x15
	reqGetEventState = 0x16
	reqSetChars      = 0x19
	reqGetBaudRate   = 0x1D
	reqSetBaudRate   = 0x1E // Preferred baud-rate control on CP2102+; sends a u32 baud value.
	reqVendorSpecific = 0xFF
)

// ifcEnable values for reqIfcEnable.
const (
	ifcDisable uint16 = 0x00
	ifcEnable  uint16 = 0x01
)

// setMhs (reqSetMhs) packs DTR/RTS into a u16: low byte = new
// values, high byte = mask of which bits to change.
const (
	mhsDTR     uint16 = 0x0001
	mhsRTS     uint16 = 0x0002
	mhsDTRMask uint16 = 0x0100
	mhsRTSMask uint16 = 0x0200
)

// getMdmSts result bits (one byte).
const (
	mdmDTR uint8 = 0x01
	mdmRTS uint8 = 0x02
	mdmCTS uint8 = 0x10
	mdmDSR uint8 = 0x20
	mdmRI  uint8 = 0x40
	mdmDCD uint8 = 0x80
)

// setLineCtl packs data bits / parity / stop bits into a u16.
// Bits 0–3: stop bits (0 = 1, 1 = 1.5, 2 = 2).
// Bits 4–7: parity (0 none, 1 odd, 2 even, 3 mark, 4 space).
// Bits 8–11: data bits (5, 6, 7, or 8).
// The remaining bits are reserved.
