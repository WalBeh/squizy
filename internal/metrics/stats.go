package metrics

import (
	"math"
	"sort"
	"time"
)

// Dist is a summary of a numeric distribution.
type Dist struct {
	N                  int
	Mean               float64
	Min, P50, P90, P99 float64
	Max                float64
}

// summarize computes a Dist from raw float samples (already in the desired unit).
func summarize(xs []float64) Dist {
	d := Dist{N: len(xs)}
	if len(xs) == 0 {
		return d
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	var sum float64
	for _, x := range sorted {
		sum += x
	}
	d.Mean = sum / float64(len(sorted))
	d.Min = sorted[0]
	d.Max = sorted[len(sorted)-1]
	d.P50 = percentile(sorted, 0.50)
	d.P90 = percentile(sorted, 0.90)
	d.P99 = percentile(sorted, 0.99)
	return d
}

// percentile uses nearest-rank on a pre-sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// durDist summarizes durations as seconds, ignoring zero/negative entries.
func durDist(ds []time.Duration) Dist {
	xs := make([]float64, 0, len(ds))
	for _, d := range ds {
		if d > 0 {
			xs = append(xs, d.Seconds())
		}
	}
	return summarize(xs)
}
