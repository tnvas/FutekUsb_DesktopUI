package processing

import (
	"testing"
	"time"
)

func reading(n float64) Reading {
	return Reading{At: time.Now(), Newtons: n}
}

func TestDespikerRejectsIsolatedSpike(t *testing.T) {
	var dropped []float64
	d := NewDespiker(func(r Reading) { dropped = append(dropped, r.Newtons) })

	// A dropped decimal point turning "25.21" into "25210": one wildly
	// different sample sandwiched between two that agree with each other.
	inputs := []float64{91.2, 91.3, 112139.7, 91.5, 91.4}
	var out []float64
	for _, n := range inputs {
		if r, ok := d.Push(reading(n)); ok {
			out = append(out, r.Newtons)
		}
	}
	if r, ok := d.Flush(); ok {
		out = append(out, r.Newtons)
	}

	want := []float64{91.2, 91.3, 91.5, 91.4}
	if len(out) != len(want) {
		t.Fatalf("output = %v, want %v", out, want)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("output[%d] = %v, want %v", i, out[i], want[i])
		}
	}
	if len(dropped) != 1 || dropped[0] != 112139.7 {
		t.Errorf("dropped = %v, want [112139.7]", dropped)
	}
}

func TestDespikerKeepsSustainedFastEvent(t *testing.T) {
	// A real recoil-style impulse: several consecutive samples jump and
	// stay elevated together. None of these should be treated as isolated
	// spikes, since each has a similarly-elevated neighbour.
	var dropped []float64
	d := NewDespiker(func(r Reading) { dropped = append(dropped, r.Newtons) })

	inputs := []float64{10, 10, 4500, 4800, 4700, 11, 10}
	var out []float64
	for _, n := range inputs {
		if r, ok := d.Push(reading(n)); ok {
			out = append(out, r.Newtons)
		}
	}
	if r, ok := d.Flush(); ok {
		out = append(out, r.Newtons)
	}

	if len(dropped) != 0 {
		t.Errorf("a sustained multi-sample event should never be dropped, got dropped=%v", dropped)
	}
	if len(out) != len(inputs) {
		t.Fatalf("output = %v (len %d), want all %d samples kept", out, len(out), len(inputs))
	}
}

func TestDespikerPassesNormalNoise(t *testing.T) {
	var dropped []float64
	d := NewDespiker(func(r Reading) { dropped = append(dropped, r.Newtons) })

	inputs := []float64{20.1, 20.3, 19.9, 20.4, 20.0, 20.2}
	var out []float64
	for _, n := range inputs {
		if r, ok := d.Push(reading(n)); ok {
			out = append(out, r.Newtons)
		}
	}
	if r, ok := d.Flush(); ok {
		out = append(out, r.Newtons)
	}
	if len(dropped) != 0 {
		t.Errorf("normal noise should never be dropped, got %v", dropped)
	}
	if len(out) != len(inputs) {
		t.Errorf("expected all %d samples through, got %d", len(inputs), len(out))
	}
}
