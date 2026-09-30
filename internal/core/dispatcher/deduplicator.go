package dispatcher

import (
	"sync"
	"time"
)

// Deduplicator prevents loops using eventID + ruleID + fromNodeID + toNodeID.
// Entries move through pending -> committed so queue rejection can be rolled
// back without opening a concurrent duplicate-submission window.
type Deduplicator struct {
	mu       sync.Mutex
	seen     map[string]*dedupEntry
	window   time.Duration
	cleanup  *time.Ticker
	stopChan chan struct{}
	stopOnce sync.Once
}

type dedupEntry struct {
	markedAt  time.Time
	pending   bool
	committed bool
	done      chan struct{}
}

// DedupReservation must be finalized exactly once with Commit or Rollback.
type DedupReservation struct {
	dedup *Deduplicator
	key   string
	entry *dedupEntry
	once  sync.Once
}

func NewDeduplicator(window time.Duration) *Deduplicator {
	if window <= 0 {
		window = time.Second
	}
	d := &Deduplicator{
		seen:     make(map[string]*dedupEntry),
		window:   window,
		stopChan: make(chan struct{}),
	}
	d.cleanup = time.NewTicker(5 * time.Second)
	go d.cleanupLoop()
	return d
}

// Reserve atomically reserves a delivery key. If another delivery for the same
// key is still being submitted, Reserve waits for its outcome: a committed
// predecessor is a duplicate, while a rolled-back predecessor may be retried.
func (d *Deduplicator) Reserve(ruleID, fromNodeID, toNodeID, eventID string) (*DedupReservation, bool) {
	if d == nil {
		return nil, false
	}
	key := d.makeKey(ruleID, fromNodeID, toNodeID, eventID)
	for {
		now := time.Now()
		d.mu.Lock()
		entry, exists := d.seen[key]
		if exists && entry.pending {
			done := entry.done
			d.mu.Unlock()
			<-done
			continue
		}
		if exists && entry.committed && now.Sub(entry.markedAt) < d.window {
			d.mu.Unlock()
			return nil, true
		}
		if exists {
			delete(d.seen, key)
		}
		entry = &dedupEntry{markedAt: now, pending: true, done: make(chan struct{})}
		d.seen[key] = entry
		d.mu.Unlock()
		return &DedupReservation{dedup: d, key: key, entry: entry}, false
	}
}

func (r *DedupReservation) Commit() {
	if r == nil || r.dedup == nil {
		return
	}
	r.once.Do(func() {
		r.dedup.mu.Lock()
		if current := r.dedup.seen[r.key]; current == r.entry {
			r.entry.pending = false
			r.entry.committed = true
			r.entry.markedAt = time.Now()
			close(r.entry.done)
		}
		r.dedup.mu.Unlock()
	})
}

func (r *DedupReservation) Rollback() {
	if r == nil || r.dedup == nil {
		return
	}
	r.once.Do(func() {
		r.dedup.mu.Lock()
		if current := r.dedup.seen[r.key]; current == r.entry {
			delete(r.dedup.seen, r.key)
			close(r.entry.done)
		}
		r.dedup.mu.Unlock()
	})
}

// CheckAndMark is retained for internal compatibility. Submission paths should
// use Reserve and finalize only after the scheduler accepts the task.
func (d *Deduplicator) CheckAndMark(ruleID, fromNodeID, toNodeID, eventID string) bool {
	reservation, duplicate := d.Reserve(ruleID, fromNodeID, toNodeID, eventID)
	if duplicate {
		return true
	}
	reservation.Commit()
	return false
}

func (d *Deduplicator) makeKey(ruleID, fromNodeID, toNodeID, eventID string) string {
	return ruleID + ":" + fromNodeID + ":" + toNodeID + ":" + eventID
}

func (d *Deduplicator) cleanupLoop() {
	for {
		select {
		case <-d.cleanup.C:
			d.cleanupExpired()
		case <-d.stopChan:
			return
		}
	}
}

func (d *Deduplicator) cleanupExpired() {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	for key, entry := range d.seen {
		if !entry.pending && now.Sub(entry.markedAt) >= d.window {
			delete(d.seen, key)
		}
	}
}

func (d *Deduplicator) Stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() {
		if d.cleanup != nil {
			d.cleanup.Stop()
		}
		close(d.stopChan)
	})
}
