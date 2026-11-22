package processing

import (
	"math"
	"testing"
	"time"

	"futek_usb220/internal/physics"
)

func TestParseLine(t *testing.T) {
	cases := []struct {
		in    string
		value float64
		unit  string
		ok    bool
		err   bool
	}{
		{"+20.5320     lbs\r\n", 20.5320, physics.LBS, true, false},
		{"123.456\r\n", 123.456, "", true, false},
		{"+00123.4 kg", 123.4, physics.KG, true, false},
		{"-2.5 N", -2.5, physics.N, true, false},
		{"", 0, "", false, false},
		{"----", 0, "", false, false},
		{"Error 42", 0, "", false, true},
		{"1.2e3 gf", 1200, physics.G, true, false},
	}
	for _, c := range cases {
		v, unit, ok, err := ParseLine(c.in)
		if ok != c.ok || (err != nil) != c.err {
			t.Errorf("ParseLine(%q) = (%v,%v,%v,%v), want ok=%v err=%v", c.in, v, unit, ok, err, c.ok, c.err)
			continue
		}
		if ok && math.Abs(v-c.value) > 1e-9 {
			t.Errorf("ParseLine(%q) value = %v, want %v", c.in, v, c.value)
		}
		if ok && unit != c.unit {
			t.Errorf("ParseLine(%q) unit = %q, want %q", c.in, unit, c.unit)
		}
	}
}

func TestPipelineTareAndStats(t *testing.T) {
	p := NewPipeline(physics.N)

	// EMA is seeded with the first sample: constant input stays constant.
	at := time.Now()
	var last Sample
	for i := 0; i < 50; i++ {
		last = p.Process(at.Add(time.Duration(i)*time.Millisecond), 10, physics.N)
	}
	if math.Abs(last.Newtons-10) > 1e-9 {
		t.Fatalf("constant input should read 10 N, got %v", last.Newtons)
	}

	done := make(chan TareResult, 1)
	p.StartTare(func(r TareResult) { done <- r })
	for i := 0; i < defaultTareSamples; i++ {
		last = p.Process(at.Add(time.Duration(100+i)*time.Millisecond), 10, physics.N)
	}
	select {
	case res := <-done:
		if math.Abs(res.MeanNewtons-10) > 1e-6 {
			t.Errorf("tare mean = %v, want 10", res.MeanNewtons)
		}
	default:
		t.Fatal("tare did not complete")
	}
	if math.Abs(last.Newtons) > 1e-6 {
		t.Errorf("after tare, constant input should read ~0, got %v", last.Newtons)
	}

	stats := p.Stats()
	if stats.Count == 0 {
		t.Error("stats should count samples")
	}
	p.ResetStats()
	if p.Stats().Count != 0 {
		t.Error("ResetStats should zero the counter")
	}
}

func TestPipelineUsesDeviceUnit(t *testing.T) {
	// A device streaming lbs should be interpreted correctly without any
	// calibration remap: 10 lbf ≈ 44.482 N.
	p := NewPipeline(physics.N)
	at := time.Now()
	s := p.Process(at, 10, physics.LBS)
	if math.Abs(s.Newtons-44.482216152605) > 1e-6 {
		t.Errorf("10 lbf should read ~44.48 N, got %v", s.Newtons)
	}
}

func TestPipelineFallbackUnit(t *testing.T) {
	// An unrecognised/missing unit label falls back to the configured unit.
	p := NewPipeline(physics.KG)
	at := time.Now()
	s := p.Process(at, 1, "")
	if math.Abs(s.Newtons-9.80665) > 1e-6 {
		t.Errorf("fallback to kg should read ~9.807 N for 1 kgf, got %v", s.Newtons)
	}
}

func TestChartBuffer(t *testing.T) {
	cb := NewChartBuffer(time.Minute, 50*time.Millisecond)
	base := time.Now().Truncate(time.Minute)
	for i := 0; i < 100; i++ {
		cb.Add(Sample{At: base.Add(time.Duration(i*10) * time.Millisecond), Newtons: float64(i)})
	}
	buckets := cb.Snapshot(time.Minute)
	if len(buckets) == 0 {
		t.Fatal("expected buckets")
	}
	// 100 samples at 10ms spacing = 1s span = 20 buckets of 50ms.
	if len(buckets) != 20 {
		t.Errorf("expected 20 buckets, got %d", len(buckets))
	}
	first := buckets[0]
	if first.Min != 0 || first.Max != 4 || first.N != 5 {
		t.Errorf("first bucket = %+v, want min 0 max 4 n 5", first)
	}
	for i := 1; i < len(buckets); i++ {
		if !buckets[i].Start.After(buckets[i-1].Start) {
			t.Fatalf("buckets not in order at %d", i)
		}
	}

	cb.Reset()
	if got := cb.Snapshot(time.Minute); len(got) != 0 {
		t.Errorf("after reset expected no buckets, got %d", len(got))
	}
}

func TestPhysicsConversions(t *testing.T) {
	// 1 kgf = 9.80665 N
	if v, err := physics.ToNewtons(physics.KG, 1); err != nil || math.Abs(v-9.80665) > 1e-9 {
		t.Errorf("1 kgf = %v N (err %v)", v, err)
	}
	// Round trip
	v, err := physics.Convert(physics.LBS, physics.KG, 10)
	if err != nil {
		t.Fatal(err)
	}
	back, _ := physics.Convert(physics.KG, physics.LBS, v)
	if math.Abs(back-10) > 1e-9 {
		t.Errorf("lbs->kg->lbs round trip = %v, want 10", back)
	}
	// Legacy "os" spelling from old databases still converts.
	if _, err := physics.ToNewtons("os", 1); err != nil {
		t.Errorf("legacy unit 'os' should be accepted: %v", err)
	}
	if _, err := physics.ToNewtons("bogus", 1); err == nil {
		t.Error("bogus unit should error")
	}
}
