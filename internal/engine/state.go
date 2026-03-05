package engine

import (
	"sort"
	"strings"
)

// BufferEntry is one deduplicated line with ordering metadata.
type BufferEntry struct {
	Line     string
	Key      string
	Sequence uint64
	Index    int
}

// OrderedSetBuffer stores unique lines while preserving deterministic order.
type OrderedSetBuffer struct {
	entries   []BufferEntry
	seen      map[string]struct{}
	nextIdx   int
	sorted    bool
	joined    string
	joinedSet bool
}

// NewOrderedSetBuffer creates an empty deduping ordered buffer.
func NewOrderedSetBuffer() *OrderedSetBuffer {
	return &OrderedSetBuffer{
		entries: make([]BufferEntry, 0, 64),
		seen:    make(map[string]struct{}, 64),
		sorted:  true,
	}
}

// Add inserts a line keyed by its dedupe key and sequence.
// It returns false when the line is empty or key already exists.
func (b *OrderedSetBuffer) Add(line, key string, seq uint64) bool {
	if line == "" {
		return false
	}
	if _, ok := b.seen[key]; ok {
		return false
	}
	entry := BufferEntry{
		Line:     line,
		Key:      key,
		Sequence: seq,
		Index:    b.nextIdx,
	}
	b.seen[key] = struct{}{}
	b.entries = append(b.entries, entry)
	if len(b.entries) > 1 {
		prev := b.entries[len(b.entries)-2]
		// Keep sort disabled only when order is already deterministic by sequence/index.
		if entry.Sequence < prev.Sequence || (entry.Sequence == prev.Sequence && entry.Index < prev.Index) {
			b.sorted = false
		}
	}
	b.joinedSet = false
	b.nextIdx++
	return true
}

func (b *OrderedSetBuffer) ensureSorted() {
	if b.sorted || len(b.entries) <= 1 {
		return
	}
	sort.Slice(b.entries, func(i, j int) bool {
		if b.entries[i].Sequence != b.entries[j].Sequence {
			return b.entries[i].Sequence < b.entries[j].Sequence
		}
		return b.entries[i].Index < b.entries[j].Index
	})
	b.sorted = true
}

// Lines returns buffered lines sorted by sequence then insertion index.
func (b *OrderedSetBuffer) Lines() []string {
	b.ensureSorted()
	lines := make([]string, len(b.entries))
	for i, e := range b.entries {
		lines[i] = e.Line
	}
	return lines
}

// Joined returns the sorted buffered lines joined as one string.
func (b *OrderedSetBuffer) Joined() string {
	b.ensureSorted()
	if b.joinedSet {
		return b.joined
	}
	var sb strings.Builder
	for _, e := range b.entries {
		sb.WriteString(e.Line)
	}
	b.joined = sb.String()
	b.joinedSet = true
	return b.joined
}

// Len returns the number of unique entries in the buffer.
func (b *OrderedSetBuffer) Len() int {
	return len(b.entries)
}

// Clear resets buffered entries and dedupe state.
func (b *OrderedSetBuffer) Clear() {
	b.entries = b.entries[:0]
	clear(b.seen)
	b.nextIdx = 0
	b.sorted = true
	b.joined = ""
	b.joinedSet = false
}
