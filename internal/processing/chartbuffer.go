package processing

import (
	"sync"
	"time"
)

// Bucket aggregates all samples that fell into one short time slot. The live
// chart draws the min–max envelope plus the mean, which stays cheap to render
// no matter how high the sample rate is.
type Bucket struct {
	Start time.Time
	Min   float64
	Max   float64
	Sum   float64
	N     int
}

// Mean returns the average value in the bucket.
func (b Bucket) Mean() float64 {
	if b.N == 0 {
		return 0
	}
	return b.Sum / float64(b.N)
}

// ChartBuffer keeps a rolling window of bucketed samples for the live chart.
// Safe for concurrent use.
type ChartBuffer struct {
	mu       sync.Mutex
	bucketMS int64
	buckets  []Bucket // ring
	head     int      // index of the newest bucket
	size     int      // number of valid buckets
}

// NewChartBuffer creates a buffer holding `window` of data aggregated into
// buckets of `bucket` duration.
func NewChartBuffer(window, bucket time.Duration) *ChartBuffer {
	n := int(window / bucket)
	if n < 2 {
		n = 2
	}
	return &ChartBuffer{
		bucketMS: bucket.Milliseconds(),
		buckets:  make([]Bucket, n),
	}
}

// Add folds a sample into the current bucket, advancing the ring as needed.
func (c *ChartBuffer) Add(s Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()

	start := s.At.Truncate(time.Duration(c.bucketMS) * time.Millisecond)
	if c.size == 0 || !c.buckets[c.head].Start.Equal(start) {
		c.head = (c.head + 1) % len(c.buckets)
		c.buckets[c.head] = Bucket{Start: start, Min: s.Newtons, Max: s.Newtons}
		if c.size < len(c.buckets) {
			c.size++
		}
	}
	b := &c.buckets[c.head]
	if s.Newtons < b.Min {
		b.Min = s.Newtons
	}
	if s.Newtons > b.Max {
		b.Max = s.Newtons
	}
	b.Sum += s.Newtons
	b.N++
}

// Reset discards all buffered data.
func (c *ChartBuffer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.size = 0
	c.head = 0
}

// Snapshot returns the buckets from the last `window`, oldest first.
func (c *ChartBuffer) Snapshot(window time.Duration) []Bucket {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.size == 0 {
		return nil
	}
	cutoff := c.buckets[c.head].Start.Add(-window)
	out := make([]Bucket, 0, c.size)
	// Walk from oldest to newest.
	for i := c.size - 1; i >= 0; i-- {
		idx := (c.head - i + len(c.buckets)*2) % len(c.buckets)
		b := c.buckets[idx]
		if b.N == 0 || b.Start.Before(cutoff) {
			continue
		}
		out = append(out, b)
	}
	return out
}
