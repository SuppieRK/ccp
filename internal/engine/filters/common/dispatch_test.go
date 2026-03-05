package common

import "testing"

func TestParseDispatchMap(t *testing.T) {
	got := ParseDispatchMap("find|max=100|hidden=1|mode = run |bad")
	if got["max"] != "100" {
		t.Fatalf("max = %q, want 100", got["max"])
	}
	if got["hidden"] != "1" {
		t.Fatalf("hidden = %q, want 1", got["hidden"])
	}
	if got["mode"] != "run" {
		t.Fatalf("mode = %q, want run", got["mode"])
	}
	if _, ok := got["bad"]; ok {
		t.Fatalf("unexpected key for malformed pair")
	}
}

func TestDispatchValue(t *testing.T) {
	if got := DispatchValue("docker logs|container=api", "container"); got != "api" {
		t.Fatalf("container = %q, want api", got)
	}
	if got := DispatchValue("docker logs|container=api", "missing"); got != "" {
		t.Fatalf("missing = %q, want empty", got)
	}
}

func TestParseBool01(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"0", false},
		{"false", false},
	} {
		if got := ParseBool01(tc.in); got != tc.want {
			t.Fatalf("ParseBool01(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := ParsePositiveInt("15", 7); got != 15 {
		t.Fatalf("ParsePositiveInt(valid) = %d, want 15", got)
	}
	if got := ParsePositiveInt("0", 7); got != 7 {
		t.Fatalf("ParsePositiveInt(zero) = %d, want fallback", got)
	}
	if got := ParsePositiveInt("bad", 7); got != 7 {
		t.Fatalf("ParsePositiveInt(invalid) = %d, want fallback", got)
	}
}
