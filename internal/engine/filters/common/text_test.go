package common

import "testing"

func TestTruncateWithSuffix(t *testing.T) {
	cases := []struct {
		name string
		max  int
		want string
	}{
		{name: "exact", max: 6, want: "abcdef"},
		{name: "truncated", max: 3, want: "abc..."},
		{name: "negative-max", max: -1, want: "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateWithSuffix("abcdef", tc.max, "..."); got != tc.want {
				t.Fatalf("TruncateWithSuffix(..., %d, ...) = %q, want %q", tc.max, got, tc.want)
			}
		})
	}
}

func TestLowerTrim(t *testing.T) {
	if got := LowerTrim("  HeLLo \n"); got != "hello" {
		t.Fatalf("LowerTrim = %q, want hello", got)
	}
}
