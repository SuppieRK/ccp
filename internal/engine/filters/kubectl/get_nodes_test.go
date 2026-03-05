package kubectlfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGetNodesMetadata(t *testing.T) {
	f := NewKubectlGetNodesFilter()
	if f.Tool() != "kubectl get nodes" {
		t.Fatalf("unexpected tool: %q", f.Tool())
	}
}

func TestGetNodesCompactionCases(t *testing.T) {
	f := NewKubectlGetNodesFilter()
	cases := []struct {
		name         string
		lines        []string
		wantContains []string
		wantMissing  []string
	}{
		{
			name: "keeps-notready-and-compacts-ready",
			lines: []string{
				"NAME STATUS ROLES AGE VERSION",
				"node-a Ready worker 3d v1.29.0",
				"node-b Ready worker 3d v1.29.0",
				"node-c NotReady worker 2m v1.29.0",
			},
			wantContains: []string{"NotReady", "[2] nodes: Ready worker v1.29.0"},
		},
		{
			name: "all-ready-summary",
			lines: []string{
				"NAME STATUS ROLES AGE VERSION",
				"node-a Ready worker 3d v1.29.0",
				"node-b Ready worker 3d v1.29.0",
				"node-c Ready worker 2m v1.29.0",
			},
			wantContains: []string{"[3] nodes: Ready worker v1.29.0"},
			wantMissing:  []string{"NotReady"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := decisionAtEOF(f, "kubectl get nodes", tc.lines...)
			for _, want := range tc.wantContains {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got %q", want, d.Output)
				}
			}
			for _, missing := range tc.wantMissing {
				if strings.Contains(d.Output, missing) {
					t.Fatalf("expected output to omit %q, got %q", missing, d.Output)
				}
			}
		})
	}
}

func TestGetNodesUnknownHeaderFallsBackToPassthrough(t *testing.T) {
	f := NewKubectlGetNodesFilter()
	d := decisionAtEOF(
		f,
		"kubectl get nodes",
		"NOM NOM",
		"node-a Ready worker",
	)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on unknown header fallback, got %q", d.Action)
	}
	if d.Output != "NOM NOM\nnode-a Ready worker\n" {
		t.Fatalf("expected passthrough output on unknown header, got %q", d.Output)
	}
}
