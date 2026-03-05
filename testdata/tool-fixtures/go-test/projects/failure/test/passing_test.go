package test

import (
	"fmt"
	"testing"
)

func TestFakePassingMath(t *testing.T) {
	fmt.Println("running fake passing math test")
	if got := (2 * 3) + 4; got != 10 {
		t.Fatalf("unexpected result: %d", got)
	}
}

func TestFakePassingSliceLen(t *testing.T) {
	t.Log("running fake passing slice test")
	items := []string{"a", "b", "c"}
	if len(items) != 3 {
		t.Fatalf("unexpected length: %d", len(items))
	}
}
