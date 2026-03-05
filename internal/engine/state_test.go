package engine

import "testing"

const joinedAB = "a\nb\n"

func TestOrderedSetBufferSequenceOrdering(t *testing.T) {
	b := NewOrderedSetBuffer()
	_ = b.Add("b\n", "b", 2)
	_ = b.Add("a\n", "a", 1)
	if got := b.Joined(); got != joinedAB {
		t.Fatalf("unexpected ordering: %q", got)
	}
}

func TestOrderedSetBufferKeyReuseDropped(t *testing.T) {
	b := NewOrderedSetBuffer()
	if !b.Add("x\n", "same", 1) {
		t.Fatal("expected first add")
	}
	if b.Add("y\n", "same", 2) {
		t.Fatal("expected duplicate key to be dropped")
	}
}

func TestOrderedSetBufferJoinedCacheInvalidatedOnAddAndClear(t *testing.T) {
	b := NewOrderedSetBuffer()
	_ = b.Add("a\n", "a", 1)
	if got := b.Joined(); got != "a\n" {
		t.Fatalf("joined = %q, want %q", got, "a\n")
	}
	_ = b.Add("b\n", "b", 2)
	if got := b.Joined(); got != joinedAB {
		t.Fatalf("joined after add = %q, want %q", got, joinedAB)
	}
	b.Clear()
	if got := b.Joined(); got != "" {
		t.Fatalf("joined after clear = %q, want empty", got)
	}
}
