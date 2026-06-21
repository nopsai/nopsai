package systemlogs

import "time"

type ringBuffer struct {
	entries  []Entry
	capacity int
	maxAge   time.Duration
}

func newRingBuffer(capacity int, maxAge time.Duration) *ringBuffer {
	if capacity <= 0 {
		capacity = 10_000
	}
	return &ringBuffer{capacity: capacity, maxAge: maxAge}
}

func (r *ringBuffer) append(entry Entry, now time.Time) {
	r.prune(now)
	r.entries = append(r.entries, entry)
	if overflow := len(r.entries) - r.capacity; overflow > 0 {
		copy(r.entries, r.entries[overflow:])
		r.entries = r.entries[:r.capacity]
	}
}

func (r *ringBuffer) tail(limit int, now time.Time) []Entry {
	r.prune(now)
	if limit <= 0 || limit > len(r.entries) {
		limit = len(r.entries)
	}
	return append([]Entry(nil), r.entries[len(r.entries)-limit:]...)
}

func (r *ringBuffer) after(sequence uint64, now time.Time) ([]Entry, error) {
	r.prune(now)
	if len(r.entries) == 0 {
		return nil, ErrCursorExpired
	}
	if sequence < cursorSequence(r.entries[0]) || sequence > cursorSequence(r.entries[len(r.entries)-1]) {
		return nil, ErrCursorExpired
	}
	for index, entry := range r.entries {
		if cursorSequence(entry) == sequence {
			return append([]Entry(nil), r.entries[index+1:]...), nil
		}
	}
	return nil, ErrCursorExpired
}

func (r *ringBuffer) prune(now time.Time) {
	if r.maxAge <= 0 || len(r.entries) == 0 {
		return
	}
	cutoff := now.Add(-r.maxAge)
	first := 0
	for first < len(r.entries) && r.entries[first].ObservedAt.Before(cutoff) {
		first++
	}
	if first > 0 {
		copy(r.entries, r.entries[first:])
		r.entries = r.entries[:len(r.entries)-first]
	}
}

func cursorSequence(entry Entry) uint64 {
	return entry.sequence
}
