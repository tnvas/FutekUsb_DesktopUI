package device

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
	"time"
)

// simDevice generates a synthetic sensor stream so the application can be
// developed, demonstrated and tested without hardware. It mimics the real
// USB220 wire format exactly: an already-calibrated, unit-labelled ASCII
// value per line (e.g. "+20.532     lbs\r\n").
type simDevice struct {
	mu     sync.Mutex
	closed bool
	start  time.Time
	rng    *rand.Rand
	rate   int // samples per second
	last   time.Time
}

// OpenSimulator returns a Device producing synthetic data at the given rate.
func OpenSimulator(rate int) Device {
	if rate <= 0 {
		rate = 1200
	}
	return &simDevice{
		start: time.Now(),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		rate:  rate,
		last:  time.Now(),
	}
}

func (d *simDevice) ReadLine() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, io.EOF
	}

	// Pace output to the configured sample rate.
	interval := time.Second / time.Duration(d.rate)
	next := d.last.Add(interval)
	if wait := time.Until(next); wait > 0 {
		time.Sleep(wait)
	}
	d.last = time.Now()

	// A small resting baseline (mirrors real load cells never reading exactly
	// zero) plus a slow 0-150 lb swing — roughly a person stepping on and off
	// — with realistic sensor noise.
	t := time.Since(d.start).Seconds()
	baseline := 2.1
	swing := 75.0 * (0.5 + 0.5*math.Sin(t/8.0*2*math.Pi))
	noise := d.rng.NormFloat64() * 0.15
	value := baseline + swing + noise
	return []byte(fmt.Sprintf("%+.4f     lbs\r\n", value)), nil
}

func (d *simDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

func (d *simDevice) Info() PortInfo {
	return PortInfo{
		Name:         "SIMULATED",
		Product:      "Simulated USB220",
		SerialNumber: "SIM000000",
		VID:          FutekVID,
		PID:          USB220PID,
	}
}
