package engine

import (
	"strings"
	"testing"
)

func BenchmarkEngineNextSequence(b *testing.B) {
	e := NewEngine(Config{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.nextSequence()
	}
}

func BenchmarkEngineMakeKeyMasking(b *testing.B) {
	e := NewEngine(Config{})
	f := NewNoopFilter("bench")
	line := strings.Repeat("1234567890abcdef", 64) + " 12345678901234567890 deadbeef"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.makeKey(line, f)
	}
}
