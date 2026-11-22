package processing

import "time"

// Reading is one parsed device line, already resolved to a unit and its
// Newtons equivalent, awaiting despike evaluation.
type Reading struct {
	At      time.Time
	Value   float64
	Unit    string
	Raw     string // original raw line, kept for diagnostics
	Newtons float64
}

// Despiker rejects isolated single-sample corruption (e.g. a dropped byte
// on the wire turning "25.210" into "25210") while never suppressing a
// genuine fast event, such as a recoil impulse, which keeps several
// consecutive samples elevated together.
//
// A candidate reading is rejected only if it differs hugely from BOTH its
// immediate neighbours while those neighbours closely agree with each
// other — the signature of a single bad line sandwiched between good ones.
// Output lags input by exactly one sample.
type Despiker struct {
	spikeThreshold    float64
	neighborTolerance float64

	havePrevEmitted bool
	prevEmitted     float64

	havePending bool
	pending     Reading

	onDrop func(Reading)
}

// Default thresholds, in Newtons. Legitimate sensor noise sits within a few
// N; the observed corruption (a dropped decimal point) inflates readings by
// ~1000x, so these are set to comfortably separate the two without ever
// mistaking a real multi-sample event (whose neighbour also moves) for a
// glitch.
const (
	defaultSpikeThresholdNewtons    = 2000
	defaultNeighborToleranceNewtons = 200
)

// NewDespiker creates a despiker with the default thresholds. onDrop, if
// non-nil, is invoked for every reading rejected as an isolated spike.
func NewDespiker(onDrop func(Reading)) *Despiker {
	return &Despiker{
		spikeThreshold:    defaultSpikeThresholdNewtons,
		neighborTolerance: defaultNeighborToleranceNewtons,
		onDrop:            onDrop,
	}
}

// Push feeds one new reading and returns the reading that should actually be
// processed, if any.
func (d *Despiker) Push(r Reading) (out Reading, ok bool) {
	if !d.havePending {
		d.pending = r
		d.havePending = true
		return Reading{}, false
	}

	candidate := d.pending
	isSpike := d.havePrevEmitted &&
		absFloat(candidate.Newtons-d.prevEmitted) > d.spikeThreshold &&
		absFloat(candidate.Newtons-r.Newtons) > d.spikeThreshold &&
		absFloat(d.prevEmitted-r.Newtons) <= d.neighborTolerance

	d.pending = r
	if isSpike {
		if d.onDrop != nil {
			d.onDrop(candidate)
		}
		return Reading{}, false
	}
	d.prevEmitted = candidate.Newtons
	d.havePrevEmitted = true
	return candidate, true
}

// Flush returns any buffered reading at shutdown. There is no lookahead
// available for it, so it is always emitted rather than evaluated.
func (d *Despiker) Flush() (out Reading, ok bool) {
	if !d.havePending {
		return Reading{}, false
	}
	out, d.havePending = d.pending, false
	return out, true
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
