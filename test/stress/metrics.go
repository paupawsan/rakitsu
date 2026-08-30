//go:build stress

package stress

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects stress test measurements.
type Metrics struct {
	mu             sync.Mutex
	eventsSent     int64
	eventsReceived int64
	eventsDropped  int64
	latencies      []time.Duration
	goroutines     []int
	heapAllocs     []uint64
	errors         []string
	startTime      time.Time
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{startTime: time.Now()}
}

func (m *Metrics) AddEventSent()     { atomic.AddInt64(&m.eventsSent, 1) }
func (m *Metrics) AddEventReceived() { atomic.AddInt64(&m.eventsReceived, 1) }
func (m *Metrics) AddEventDropped()  { atomic.AddInt64(&m.eventsDropped, 1) }

func (m *Metrics) AddLatency(d time.Duration) {
	m.mu.Lock()
	m.latencies = append(m.latencies, d)
	m.mu.Unlock()
}

func (m *Metrics) AddError(err error) {
	m.mu.Lock()
	m.errors = append(m.errors, err.Error())
	m.mu.Unlock()
}

// Snapshot captures current goroutine count and heap allocation.
func (m *Metrics) Snapshot() {
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	m.mu.Lock()
	m.goroutines = append(m.goroutines, runtime.NumGoroutine())
	m.heapAllocs = append(m.heapAllocs, ms.HeapAlloc)
	m.mu.Unlock()
}

// Report generates a summary report.
type Report struct {
	TotalEventsSent int64   `json:"total_events_sent"`
	TotalReceived   int64   `json:"total_received"`
	TotalDropped    int64   `json:"total_dropped"`
	DropRate        float64 `json:"drop_rate"`
	P50Latency      string  `json:"p50_latency"`
	P95Latency      string  `json:"p95_latency"`
	P99Latency      string  `json:"p99_latency"`
	PeakGoroutines  int     `json:"peak_goroutines"`
	PeakHeapMB      float64 `json:"peak_heap_mb"`
	ErrorCount      int     `json:"error_count"`
	Duration        string  `json:"duration"`
}

func (m *Metrics) Report() *Report {
	m.mu.Lock()
	defer m.mu.Unlock()

	sent := atomic.LoadInt64(&m.eventsSent)
	recv := atomic.LoadInt64(&m.eventsReceived)
	drop := atomic.LoadInt64(&m.eventsDropped)

	r := &Report{
		TotalEventsSent: sent,
		TotalReceived:   recv,
		TotalDropped:    drop,
		ErrorCount:      len(m.errors),
		Duration:        time.Since(m.startTime).String(),
	}

	if sent > 0 {
		r.DropRate = float64(drop) / float64(sent)
	}

	if len(m.latencies) > 0 {
		sorted := make([]time.Duration, len(m.latencies))
		copy(sorted, m.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		r.P50Latency = sorted[len(sorted)*50/100].String()
		r.P95Latency = sorted[len(sorted)*95/100].String()
		r.P99Latency = sorted[len(sorted)*99/100].String()
	}

	for _, g := range m.goroutines {
		if g > r.PeakGoroutines {
			r.PeakGoroutines = g
		}
	}
	for _, h := range m.heapAllocs {
		mb := float64(h) / (1024 * 1024)
		if mb > r.PeakHeapMB {
			r.PeakHeapMB = mb
		}
	}

	return r
}

// WriteJSON writes the report to a file.
func (r *Report) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Print prints the report to stderr.
func (r *Report) Print() {
	fmt.Fprintf(os.Stderr, "\n=== Stress Test Report ===\n")
	fmt.Fprintf(os.Stderr, "Events: %d sent, %d received, %d dropped (%.2f%% drop rate)\n",
		r.TotalEventsSent, r.TotalReceived, r.TotalDropped, r.DropRate*100)
	fmt.Fprintf(os.Stderr, "Latency: p50=%s p95=%s p99=%s\n", r.P50Latency, r.P95Latency, r.P99Latency)
	fmt.Fprintf(os.Stderr, "Peak goroutines: %d, Peak heap: %.1f MB\n", r.PeakGoroutines, r.PeakHeapMB)
	fmt.Fprintf(os.Stderr, "Errors: %d, Duration: %s\n", r.ErrorCount, r.Duration)
}
