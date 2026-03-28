package engine

import (
	"strings"

	"go-command-compression-proxy/internal/contracts"
)

type BufferEntry struct {
	Stream contracts.Stream
	Line   string
}

type OrderedBuffer struct {
	entries []BufferEntry
	// seen intentionally deduplicates repeated identical retained lines within a
	// single stream. This is a product choice to keep compacted output concise;
	// identical lines across different streams are still tracked independently.
	seen map[string]struct{}
}

func NewOrderedBuffer() *OrderedBuffer {
	return &OrderedBuffer{
		entries: make([]BufferEntry, 0, 64),
		seen:    make(map[string]struct{}, 64),
	}
}

func (b *OrderedBuffer) Add(stream contracts.Stream, line string) bool {
	line = StripANSI(line)
	if strings.TrimSpace(line) == "" {
		return false
	}
	dedupeKey := string(stream) + "\x00" + line
	if _, ok := b.seen[dedupeKey]; ok {
		return false
	}
	entry := BufferEntry{
		Stream: stream,
		Line:   line,
	}
	b.seen[dedupeKey] = struct{}{}
	b.entries = append(b.entries, entry)
	return true
}

func (b *OrderedBuffer) RemoveLast(stream contracts.Stream, count int) int {
	if count <= 0 || len(b.entries) == 0 {
		return 0
	}
	removed := 0
	for i := len(b.entries) - 1; i >= 0 && removed < count; i-- {
		entry := b.entries[i]
		if stream != contracts.StreamCombined && entry.Stream != stream {
			continue
		}
		delete(b.seen, string(entry.Stream)+"\x00"+entry.Line)
		b.entries = append(b.entries[:i], b.entries[i+1:]...)
		removed++
	}
	return removed
}

func (b *OrderedBuffer) Lines(stream contracts.Stream) []string {
	lines := make([]string, 0, len(b.entries))
	for _, entry := range b.entries {
		if stream == contracts.StreamCombined || entry.Stream == stream {
			lines = append(lines, entry.Line)
		}
	}
	return lines
}

func (b *OrderedBuffer) Entries() []BufferEntry {
	entries := make([]BufferEntry, len(b.entries))
	copy(entries, b.entries)
	return entries
}

func (b *OrderedBuffer) Count(stream contracts.Stream) int {
	if stream == contracts.StreamCombined {
		return len(b.entries)
	}
	count := 0
	for _, entry := range b.entries {
		if entry.Stream == stream {
			count++
		}
	}
	return count
}

func (b *OrderedBuffer) Joined(stream contracts.Stream) string {
	var sb strings.Builder
	for _, entry := range b.entries {
		if stream == contracts.StreamCombined || entry.Stream == stream {
			sb.WriteString(entry.Line)
		}
	}
	return sb.String()
}

func (b *OrderedBuffer) Len() int {
	return len(b.entries)
}

func (b *OrderedBuffer) Clear() {
	b.entries = b.entries[:0]
	clear(b.seen)
}
