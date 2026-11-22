package processing

import (
	"math"
	"sync"
	"time"

	"futek_usb220/internal/physics"
)

// Sample is one calibrated, filtered sensor reading. Values are always kept
// in Newtons internally; display/export units are converted at the edges.
type Sample struct {
	At      time.Time
	Newtons float64
}

// TareResult reports the outcome of a tare (zeroing) operation.
type TareResult struct {
	MeanNewtons float64 // new zero point
	Noise       float64 // ±2σ spread observed while taring, in N
}

// Stats summarises the live stream since the last reset.
type Stats struct {
	Current float64 // most recent value (N)
	Min     float64
	Max     float64
	Avg     float64
	Count   int64
	Rate    int // samples per second, measured
}

const defaultEMAPeriod = 20
const defaultTareSamples = 100

// Pipeline smooths and tares the device's already-calibrated sample stream
// and maintains live statistics. Safe for concurrent use.
type Pipeline struct {
	mu sync.Mutex

	// fallbackUnit is used when a device line doesn't carry a recognisable
	// unit label. The USB220 always labels its output, so this only matters
	// for unusual devices/firmware.
	fallbackUnit string

	emaAlpha float64
	ema      float64
	emaSet   bool

	tare       float64
	taring     bool
	tareBuf    []float64
	onTareDone func(TareResult)

	stats      Stats
	sum        float64
	statsEmpty bool

	// samples-per-second measurement
	rateSecond time.Time
	rateCount  int
}

// NewPipeline creates a pipeline. fallbackUnit is assumed for lines whose
// unit label the parser didn't recognise; pass "" for Newtons.
func NewPipeline(fallbackUnit string) *Pipeline {
	if fallbackUnit == "" {
		fallbackUnit = physics.N
	}
	return &Pipeline{
		fallbackUnit: fallbackUnit,
		emaAlpha:     2.0 / (float64(defaultEMAPeriod) + 1.0),
		statsEmpty:   true,
	}
}

// SetFallbackUnit changes the unit assumed for unlabelled lines.
func (p *Pipeline) SetFallbackUnit(unit string) {
	if unit == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fallbackUnit = unit
}

// StartTare begins collecting samples to establish a new zero point. done is
// called (from the processing goroutine) when enough samples are gathered.
func (p *Pipeline) StartTare(done func(TareResult)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taring = true
	p.tareBuf = p.tareBuf[:0]
	p.onTareDone = done
}

// ClearTare removes the current zero offset.
func (p *Pipeline) ClearTare() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tare = 0
}

// ResetStats clears min/max/avg tracking.
func (p *Pipeline) ResetStats() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats = Stats{Rate: p.stats.Rate, Current: p.stats.Current}
	p.sum = 0
	p.statsEmpty = true
}

// Stats returns a snapshot of the live statistics.
func (p *Pipeline) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// Process smooths and tares one already-calibrated device reading. unit is
// the label parsed from the device line ("" if unrecognised, in which case
// the configured fallback unit is used).
func (p *Pipeline) Process(at time.Time, value float64, unit string) Sample {
	p.mu.Lock()

	if unit == "" {
		unit = p.fallbackUnit
	}
	newtons, err := physics.ToNewtons(unit, value)
	if err != nil {
		newtons = value
	}

	// Exponential smoothing, seeded with the first value so the reading does
	// not ramp up from zero after (re)connection.
	if !p.emaSet {
		p.ema = newtons
		p.emaSet = true
	} else {
		p.ema = newtons*p.emaAlpha + p.ema*(1-p.emaAlpha)
	}

	var tareDone func(TareResult)
	var tareRes TareResult
	if p.taring {
		p.tareBuf = append(p.tareBuf, p.ema)
		if len(p.tareBuf) >= defaultTareSamples {
			mean, sd := meanStd(p.tareBuf)
			p.tare = mean
			p.taring = false
			tareDone = p.onTareDone
			p.onTareDone = nil
			tareRes = TareResult{MeanNewtons: mean, Noise: 2 * sd}
			// Zeroing changes the value scale; restart the min/max tracking.
			p.stats = Stats{Rate: p.stats.Rate}
			p.sum = 0
			p.statsEmpty = true
		}
	}

	value = p.ema - p.tare

	// Statistics
	p.stats.Current = value
	p.stats.Count++
	p.sum += value
	p.stats.Avg = p.sum / float64(p.stats.Count)
	if p.statsEmpty || value < p.stats.Min {
		p.stats.Min = value
	}
	if p.statsEmpty || value > p.stats.Max {
		p.stats.Max = value
	}
	p.statsEmpty = false

	// Measured sample rate: count arrivals per wall-clock second.
	sec := at.Truncate(time.Second)
	if sec.Equal(p.rateSecond) {
		p.rateCount++
	} else {
		if !p.rateSecond.IsZero() {
			p.stats.Rate = p.rateCount
		}
		p.rateSecond = sec
		p.rateCount = 1
	}

	p.mu.Unlock()
	if tareDone != nil {
		tareDone(tareRes)
	}
	return Sample{At: at, Newtons: value}
}

// Taring reports whether a tare collection is in progress.
func (p *Pipeline) Taring() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.taring
}

func meanStd(xs []float64) (mean, sd float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean = sum / n
	if n < 2 {
		return mean, 0
	}
	var sq float64
	for _, x := range xs {
		d := x - mean
		sq += d * d
	}
	sd = math.Sqrt(sq / (n - 1))
	return mean, sd
}
