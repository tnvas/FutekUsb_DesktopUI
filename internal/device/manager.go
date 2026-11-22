package device

import (
	"context"
	"log"
	"sync"
	"time"
)

// RawSample is one parsed line from the device: the raw count value the
// sensor reported and the time it was read.
type RawSample struct {
	At    time.Time
	Value []byte // raw line, still to be parsed
}

// Opener creates a Device for a port. Swapped out in simulation mode.
type Opener func(info PortInfo, baud int) (Device, error)

// Manager owns the device connection lifecycle: it discovers the sensor,
// keeps the connection alive with automatic reconnection, and can pause the
// connection temporarily (the FUTEK DLL needs exclusive port access when
// changing the sample rate).
//
// Raw lines are delivered on Lines(); connection state changes on Status().
type Manager struct {
	open     Opener
	simulate bool

	mu         sync.Mutex
	dev        Device
	preferred  string // user-selected port name; empty = auto-detect
	paused     bool
	generation int // incremented on every disconnect request to stop stale readers

	lines  chan RawSample
	status chan StatusEvent
	last   StatusEvent
}

// NewManager creates a manager. If simulate is true, a synthetic device is
// used instead of scanning for hardware.
func NewManager(simulate bool) *Manager {
	m := &Manager{
		open:     OpenSerial,
		simulate: simulate,
		lines:    make(chan RawSample, 8192),
		status:   make(chan StatusEvent, 16),
	}
	return m
}

// Lines returns the stream of raw sensor lines.
func (m *Manager) Lines() <-chan RawSample { return m.lines }

// Status returns connection state change events.
func (m *Manager) Status() <-chan StatusEvent { return m.status }

// LastStatus returns the most recently published status event.
func (m *Manager) LastStatus() StatusEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// SetPreferredPort pins the connection to a specific port name ("" = auto).
// The current connection is dropped so the run loop reconnects.
func (m *Manager) SetPreferredPort(name string) {
	m.mu.Lock()
	m.preferred = name
	m.mu.Unlock()
	m.dropCurrent()
}

// PreferredPort returns the pinned port name ("" = auto-detect).
func (m *Manager) PreferredPort() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.preferred
}

// Pause closes the serial connection and stops reconnecting until Resume is
// called. Used while the DLL bridge needs the port.
func (m *Manager) Pause() {
	m.mu.Lock()
	m.paused = true
	m.mu.Unlock()
	m.dropCurrent()
	m.publish(StatusEvent{Status: StatusPaused, At: time.Now()})
}

// Resume re-enables the reconnect loop after Pause.
func (m *Manager) Resume() {
	m.mu.Lock()
	m.paused = false
	m.mu.Unlock()
}

// ConnectedPort returns the active port and whether a device is connected.
func (m *Manager) ConnectedPort() (PortInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return PortInfo{}, false
	}
	return m.dev.Info(), true
}

// Run drives discovery, reading and reconnection until ctx is cancelled.
// It is intended to run in its own goroutine.
func (m *Manager) Run(ctx context.Context) {
	defer m.closeCurrent()
	backoff := time.Second

	for ctx.Err() == nil {
		m.mu.Lock()
		paused := m.paused
		preferred := m.preferred
		m.mu.Unlock()

		if paused {
			sleepCtx(ctx, 300*time.Millisecond)
			continue
		}

		dev, err := m.connect(preferred)
		if err != nil {
			m.publish(StatusEvent{Status: StatusSearching, Err: err, At: time.Now()})
			sleepCtx(ctx, backoff)
			if backoff < 5*time.Second {
				backoff += time.Second
			}
			continue
		}
		backoff = time.Second

		m.mu.Lock()
		m.dev = dev
		m.generation++
		gen := m.generation
		m.mu.Unlock()
		m.publish(StatusEvent{Status: StatusConnected, Port: dev.Info(), At: time.Now()})

		m.readLoop(ctx, dev, gen)
		m.closeCurrent()
		if ctx.Err() == nil {
			m.publish(StatusEvent{Status: StatusSearching, At: time.Now()})
		}
	}
}

// connect finds and opens a device according to the current mode.
func (m *Manager) connect(preferred string) (Device, error) {
	if m.simulate {
		return OpenSimulator(1200), nil
	}
	var info PortInfo
	if preferred != "" {
		ports, err := ListPorts()
		if err != nil {
			return nil, err
		}
		found := false
		for _, p := range ports {
			if p.Name == preferred {
				info, found = p, true
				break
			}
		}
		if !found {
			info = PortInfo{Name: preferred}
		}
	} else {
		var ok bool
		info, ok = FindFutekPort()
		if !ok {
			return nil, errNoDevice
		}
	}
	return m.open(info, DefaultBaud)
}

// readLoop pumps lines from the device until an error or a newer generation
// (drop/pause request) invalidates it.
func (m *Manager) readLoop(ctx context.Context, dev Device, gen int) {
	for {
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		stale := m.generation != gen || m.dev != dev
		m.mu.Unlock()
		if stale {
			return
		}

		line, err := dev.ReadLine()
		if err != nil {
			log.Printf("device read ended: %v", err)
			return
		}
		sample := RawSample{At: time.Now(), Value: line}
		select {
		case m.lines <- sample:
		default:
			// Consumer stalled; drop the oldest sample to keep latency bounded.
			select {
			case <-m.lines:
			default:
			}
			select {
			case m.lines <- sample:
			default:
			}
		}
	}
}

// dropCurrent closes the active device (if any) so the run loop reconnects.
func (m *Manager) dropCurrent() {
	m.mu.Lock()
	m.generation++
	dev := m.dev
	m.dev = nil
	m.mu.Unlock()
	if dev != nil {
		dev.Close()
	}
}

func (m *Manager) closeCurrent() {
	m.mu.Lock()
	dev := m.dev
	m.dev = nil
	m.mu.Unlock()
	if dev != nil {
		dev.Close()
	}
}

func (m *Manager) publish(ev StatusEvent) {
	m.mu.Lock()
	m.last = ev
	m.mu.Unlock()
	select {
	case m.status <- ev:
	default: // UI fell behind; the latest state is still in LastStatus.
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

type noDeviceError struct{}

func (noDeviceError) Error() string { return "no FUTEK USB220 device found" }

var errNoDevice = noDeviceError{}
