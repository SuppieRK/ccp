package main

import "testing"

func TestMathAdd(t *testing.T) {
	if 2+3 != 5 {
		t.Fatal("unexpected add result")
	}
}

func TestMathMul(t *testing.T) {
	if 4*5 != 20 {
		t.Fatal("unexpected mul result")
	}
}

func TestStringLen(t *testing.T) {
	if len("benchmark") != 9 {
		t.Fatal("unexpected string length")
	}
}

func TestSliceAppend(t *testing.T) {
	items := []int{1, 2}
	items = append(items, 3)
	if len(items) != 3 || items[2] != 3 {
		t.Fatal("unexpected append result")
	}
}

func TestMapLookup(t *testing.T) {
	m := map[string]int{"ok": 1}
	if m["ok"] != 1 {
		t.Fatal("unexpected map lookup result")
	}
}

func TestRuneCountASCII(t *testing.T) {
	if len([]rune("proxy")) != 5 {
		t.Fatal("unexpected rune count")
	}
}

func TestBoolLogic(t *testing.T) {
	if 3 <= 2 {
		t.Fatal("unexpected bool logic")
	}
}

func TestLoopSum(t *testing.T) {
	sum := 0
	for i := 1; i <= 4; i++ {
		sum += i
	}
	if sum != 10 {
		t.Fatal("unexpected loop sum")
	}
}

func TestByteCompare(t *testing.T) {
	a := []byte("ccp")
	b := []byte{'c', 'x', 'p'}
	t.Log("context: byte compare baseline=ccp expected=cxp")
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("intentional mismatch at index %d", i)
		}
	}
}

func TestStructField(t *testing.T) {
	type item struct {
		Name string
	}
	v := item{Name: "fixture"}
	t.Log("context: struct field expected=fixture-bad actual=fixture")
	if v.Name != "fixture-bad" {
		t.Fatal("intentional struct mismatch")
	}
}
