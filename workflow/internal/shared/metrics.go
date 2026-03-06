package shared

import (
	"math"
	"sort"
	"sync"
)

type MetricCollector struct {
	mu      sync.Mutex
	samples []float64
	calls   int
	errors  int
}

func (m *MetricCollector) Add(ms float64, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, ms)
	m.calls++
	if !ok {
		m.errors++
	}
}

func (m *MetricCollector) Summary() (int, int, float64, float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.samples) == 0 {
		return m.calls, m.errors, 0, 0, 0
	}
	vals := append([]float64(nil), m.samples...)
	sort.Float64s(vals)
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	p50 := Percentile(vals, 50)
	p95 := Percentile(vals, 95)
	return m.calls, m.errors, sum / float64(len(vals)), p50, p95
}

func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	k := (float64(len(sorted)-1) * p) / 100.0
	lo := int(math.Floor(k))
	hi := int(math.Ceil(k))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(k-float64(lo))
}
