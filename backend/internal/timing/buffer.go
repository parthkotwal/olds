// Package timing provides a thread-safe ring buffer for collecting latency
// samples and computing percentiles for the Phase 17 /stats metrics endpoint.
package timing

import (
	"sort"
	"sync"
	"time"
)

// capacity is the maximum number of samples retained in the rolling window.
// The oldest sample is overwritten when the buffer is full.
const capacity = 1000

// Buffer is a fixed-capacity ring buffer of time.Duration samples.
//
// When the buffer is full, new samples overwrite the oldest (ring behaviour).
// Percentile() computes over the current window of up to capacity samples.
//
// Zero value is ready to use — no constructor needed.
// Do not copy a Buffer (it contains a mutex).
type Buffer struct {
	mu    sync.Mutex
	data  [capacity]time.Duration
	count int // total samples ever added (unbounded; may exceed capacity)
	pos   int // next write position in data (wraps at capacity)
}

// Add records a timing sample. Safe to call concurrently from many goroutines.
func (b *Buffer) Add(d time.Duration) {
	b.mu.Lock()
	b.data[b.pos] = d
	b.pos = (b.pos + 1) % capacity
	b.count++
	b.mu.Unlock()
}

// Percentile returns the p-th percentile of the current sample window
// (p in [0, 100]). Returns 0 if no samples have been recorded yet.
//
// The lock is released before sorting so the buffer is not blocked for the
// duration of the sort — important when callers are concurrently adding samples.
func (b *Buffer) Percentile(p float64) time.Duration {
	b.mu.Lock()
	n := b.count
	if n > capacity {
		n = capacity
	}
	if n == 0 {
		b.mu.Unlock()
		return 0
	}
	// Copy the live window before releasing the lock so the sort below
	// operates on a stable snapshot.
	snap := make([]time.Duration, n)
	copy(snap, b.data[:n])
	b.mu.Unlock()

	sort.Slice(snap, func(i, j int) bool { return snap[i] < snap[j] })

	// Nearest-rank percentile: map p ∈ [0,100] → index ∈ [0, n-1].
	idx := int(float64(n-1) * p / 100.0)
	return snap[idx]
}

// Count returns the total number of samples ever added (may exceed capacity).
func (b *Buffer) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}
