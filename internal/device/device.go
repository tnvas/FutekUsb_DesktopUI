// Package device handles discovery of and communication with the FUTEK
// USB220 serial adapter. A Device delivers raw ASCII lines from the sensor
// stream; interpretation of those lines happens in the processing package.
package device

import (
	"time"
)

// Known USB descriptor values for the FUTEK USB220 (FTDI-based).
const (
	FutekVID  = "0403" // FTDI vendor ID used by FUTEK adapters
	USB220PID = "6001" // FT232R product ID used by the USB220

	// DefaultBaud is the fixed baud rate of the USB220 virtual COM port.
	DefaultBaud = 2000000
)

// PortInfo describes a serial port found on the system.
type PortInfo struct {
	Name         string // OS port name, e.g. COM3 or /dev/ttyUSB0
	SerialNumber string
	VID          string
	PID          string
	Product      string
}

// IsFutek reports whether the port descriptors match a FUTEK USB220 adapter.
func (p PortInfo) IsFutek() bool {
	return p.VID == FutekVID && p.PID == USB220PID
}

// Label returns a human-readable description for display in the UI.
func (p PortInfo) Label() string {
	l := p.Name
	if p.Product != "" {
		l += " - " + p.Product
	}
	if p.SerialNumber != "" {
		l += " (" + p.SerialNumber + ")"
	}
	return l
}

// Device is a connected sensor data source.
type Device interface {
	// ReadLine blocks until the next ASCII line is available or an error
	// occurs (timeout, disconnect).
	ReadLine() ([]byte, error)
	// Close releases the underlying resources.
	Close() error
	// Info returns the port this device is attached to.
	Info() PortInfo
}

// Status describes the connection state shown to the user.
type Status int

const (
	StatusDisconnected Status = iota
	StatusSearching
	StatusConnected
	StatusPaused // temporarily released, e.g. while the DLL bridge talks to the device
)

func (s Status) String() string {
	switch s {
	case StatusSearching:
		return "Searching for device…"
	case StatusConnected:
		return "Connected"
	case StatusPaused:
		return "Configuring device…"
	default:
		return "Disconnected"
	}
}

// StatusEvent is emitted by the Manager whenever the connection state changes.
type StatusEvent struct {
	Status Status
	Port   PortInfo // valid when Status == StatusConnected
	Err    error    // last error, if any
	At     time.Time
}
