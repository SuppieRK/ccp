package engine

import (
	"strconv"
	"testing"
)

func BenchmarkOrderedSetBufferAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := NewOrderedSetBuffer()
		key := "k" + strconv.Itoa(i)
		line := "line-" + strconv.Itoa(i) + "\n"
		_ = buf.Add(line, key, uint64(i+1))
	}
}

func BenchmarkOrderedSetBufferJoined(b *testing.B) {
	buf := NewOrderedSetBuffer()
	for i := 0; i < 512; i++ {
		key := "k" + strconv.Itoa(i)
		line := "line-" + strconv.Itoa(i) + "\n"
		_ = buf.Add(line, key, uint64(i+1))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.Joined()
	}
}

func BenchmarkOrderedSetBufferClear(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buf := NewOrderedSetBuffer()
		for j := 0; j < 128; j++ {
			key := "k" + strconv.Itoa(j)
			line := "line-" + strconv.Itoa(j) + "\n"
			_ = buf.Add(line, key, uint64(j+1))
		}
		buf.Clear()
	}
}
