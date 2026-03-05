package common

import (
	"slices"
	"testing"
)

func TestNonEmptyLines(t *testing.T) {
	raw := "\nalpha\r\n  \n beta \n\r\ngamma\r\n"
	got := NonEmptyLines(raw)
	want := []string{"alpha", " beta ", "gamma"}
	if !slices.Equal(got, want) {
		t.Fatalf("lines mismatch: got=%#v want=%#v", got, want)
	}
}
