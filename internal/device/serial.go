package device

import (
	"bufio"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// readTimeout bounds a single blocking read so a silent device is detected
// as a stall instead of hanging forever.
const readTimeout = 2 * time.Second

// serialDevice reads newline-terminated frames from a physical serial port.
type serialDevice struct {
	port   serial.Port
	reader *bufio.Reader
	info   PortInfo
}

// OpenSerial opens the given port at the USB220 baud rate.
func OpenSerial(info PortInfo, baud int) (Device, error) {
	if baud <= 0 {
		baud = DefaultBaud
	}
	port, err := serial.Open(info.Name, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", info.Name, err)
	}
	if err := port.SetReadTimeout(readTimeout); err != nil {
		port.Close()
		return nil, fmt.Errorf("set read timeout on %s: %w", info.Name, err)
	}
	return &serialDevice{
		port:   port,
		reader: bufio.NewReaderSize(port, 4096),
		info:   info,
	}, nil
}

func (d *serialDevice) ReadLine() ([]byte, error) {
	return d.reader.ReadBytes('\n')
}

func (d *serialDevice) Close() error {
	if d.port == nil {
		return nil
	}
	err := d.port.Close()
	d.port = nil
	return err
}

func (d *serialDevice) Info() PortInfo { return d.info }

// ListPorts enumerates all serial ports, FUTEK devices first.
func ListPorts() ([]PortInfo, error) {
	seen := map[string]bool{}
	var ports []PortInfo

	detailed, err := enumerator.GetDetailedPortsList()
	if err == nil {
		for _, p := range detailed {
			ports = append(ports, PortInfo{
				Name:         p.Name,
				SerialNumber: p.SerialNumber,
				VID:          p.VID,
				PID:          p.PID,
				Product:      p.Product,
			})
			seen[p.Name] = true
		}
	}

	// Fall back to the plain port list for anything enumeration missed.
	names, err2 := serial.GetPortsList()
	if err2 == nil {
		for _, name := range names {
			if !seen[name] {
				ports = append(ports, PortInfo{Name: name})
			}
		}
	}
	if err != nil && err2 != nil {
		return nil, fmt.Errorf("list serial ports: %w", err)
	}

	sort.SliceStable(ports, func(i, j int) bool {
		fi, fj := ports[i].IsFutek(), ports[j].IsFutek()
		if fi != fj {
			return fi
		}
		return ports[i].Name < ports[j].Name
	})
	return ports, nil
}

// FindFutekPort returns the first port that identifies as a FUTEK USB220.
// If none matches by descriptor, it falls back to the first USB-serial-looking
// port so the app still works with unbranded adapters.
func FindFutekPort() (PortInfo, bool) {
	ports, err := ListPorts()
	if err != nil {
		return PortInfo{}, false
	}
	for _, p := range ports {
		if p.IsFutek() {
			return p, true
		}
	}
	for _, p := range ports {
		up := strings.ToUpper(p.Name)
		if strings.Contains(up, "USB") || strings.Contains(up, "ACM") || strings.HasPrefix(up, "COM") {
			return p, true
		}
	}
	return PortInfo{}, false
}
