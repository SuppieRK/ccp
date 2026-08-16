package engine

import (
	"bytes"
	"slices"

	"github.com/SuppieRK/cmdshape/internal/contracts"
)

type BufferEntry struct {
	Sequence    uint64
	Stream      contracts.Stream
	Original    []byte
	Transformed []byte
	Line        string
	Newline     bool
}

type OrderedBuffer struct {
	entries      []BufferEntry
	counts       map[contracts.Stream]int
	nextSequence uint64
}

func NewOrderedBuffer() *OrderedBuffer {
	return &OrderedBuffer{
		entries: make([]BufferEntry, 0, 64),
		counts:  make(map[contracts.Stream]int, 2),
	}
}

func (b *OrderedBuffer) Add(stream contracts.Stream, line string) bool {
	b.AddAt(b.nextSequence, stream, []byte(line), []byte(line))
	b.nextSequence++
	return true
}

func (b *OrderedBuffer) AddAt(sequence uint64, stream contracts.Stream, original, transformed []byte) {
	entry := newBufferEntry(sequence, stream, original, transformed)
	b.entries = append(b.entries, entry)
	b.counts[stream]++
	b.nextSequence = max(b.nextSequence, sequence+1)
}

func newBufferEntry(sequence uint64, stream contracts.Stream, original, transformed []byte) BufferEntry {
	original = bytes.Clone(original)
	transformed = bytes.Clone(transformed)
	return BufferEntry{
		Sequence:    sequence,
		Stream:      stream,
		Original:    original,
		Transformed: transformed,
		Line:        string(transformed),
		Newline:     bytes.HasSuffix(transformed, []byte{'\n'}),
	}
}

func (b *OrderedBuffer) RemoveLast(stream contracts.Stream, count int) int {
	return len(b.RemoveLastEntries(stream, count))
}

func (b *OrderedBuffer) RemoveLastEntries(stream contracts.Stream, count int) []BufferEntry {
	if count <= 0 || len(b.entries) == 0 {
		return nil
	}
	remove := make([]bool, len(b.entries))
	removed := make([]BufferEntry, 0, min(count, len(b.entries)))
	for i := len(b.entries) - 1; i >= 0 && len(removed) < count; i-- {
		entry := b.entries[i]
		if stream != contracts.StreamCombined && entry.Stream != stream {
			continue
		}
		removed = append(removed, entry)
		remove[i] = true
		b.counts[entry.Stream]--
	}
	slices.Reverse(removed)
	kept := b.entries[:0]
	for i := range b.entries {
		if !remove[i] {
			kept = append(kept, b.entries[i])
		}
	}
	clear(b.entries[len(kept):])
	b.entries = kept
	return removed
}

func (b *OrderedBuffer) Lines(stream contracts.Stream) []string {
	entries := b.Entries()
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if stream == contracts.StreamCombined || entry.Stream == stream {
			lines = append(lines, entry.Line)
		}
	}
	return lines
}

func (b *OrderedBuffer) Entries() []BufferEntry {
	entries := slices.Clone(b.entries)
	slices.SortStableFunc(entries, func(a, c BufferEntry) int {
		switch {
		case a.Sequence < c.Sequence:
			return -1
		case a.Sequence > c.Sequence:
			return 1
		default:
			return 0
		}
	})
	for i := range entries {
		entries[i].Original = bytes.Clone(entries[i].Original)
		entries[i].Transformed = bytes.Clone(entries[i].Transformed)
	}
	return entries
}

func (b *OrderedBuffer) Count(stream contracts.Stream) int {
	if stream == contracts.StreamCombined {
		return len(b.entries)
	}
	return b.counts[stream]
}

func (b *OrderedBuffer) Joined(stream contracts.Stream) string {
	var out bytes.Buffer
	for _, entry := range b.Entries() {
		if stream == contracts.StreamCombined || entry.Stream == stream {
			_, _ = out.Write(entry.Transformed)
		}
	}
	return out.String()
}

func (b *OrderedBuffer) Len() int {
	return len(b.entries)
}

func (b *OrderedBuffer) Clear() {
	clear(b.entries)
	b.entries = b.entries[:0]
	clear(b.counts)
	b.nextSequence = 0
}
