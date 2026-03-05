package common

import (
	"strings"
	"testing"
)

func TestNodeCompactOutputDedupesWarningsAndDropsProgress(t *testing.T) {
	t.Parallel()
	raw := strings.Join([]string{
		"\x1b[32m⠋ loading\x1b[0m",
		"(node:1234) ExperimentalWarning: VM Modules is an experimental feature",
		"(node:9999) ExperimentalWarning: VM Modules is an experimental feature",
		"payload line",
	}, "\n")
	out, ok := NodeCompactOutput(raw)
	if !ok {
		t.Fatal("expected compacted output")
	}
	if strings.Contains(out, "loading") {
		t.Fatalf("expected progress noise to be removed, got %q", out)
	}
	if strings.Count(out, "ExperimentalWarning") != 1 {
		t.Fatalf("expected warning dedupe, got %q", out)
	}
	if !strings.Contains(out, "[+1 similar warnings]") {
		t.Fatalf("expected similar-warning summary, got %q", out)
	}
	if !strings.Contains(out, "payload line") {
		t.Fatalf("expected payload line retained, got %q", out)
	}
}

func TestNodeCompactOutputEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		wantOut string
		wantOK  bool
	}{
		{
			name:    "low-confidence-passthrough",
			raw:     "bad\x00payload",
			wantOut: "bad\x00payload",
			wantOK:  false,
		},
		{
			name:    "all-progress-drops-to-empty",
			raw:     "⠋ loading\n\r",
			wantOut: "",
			wantOK:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := NodeCompactOutput(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("unexpected ok: got %v want %v", ok, tc.wantOK)
			}
			if out != tc.wantOut {
				t.Fatalf("unexpected output: got %q want %q", out, tc.wantOut)
			}
		})
	}
}
