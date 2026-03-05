package kubectlfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const kubectlGetPodsTool = "kubectl get pods"

func TestGetPodsMetadata(t *testing.T) {
	f := NewKubectlGetPodsFilter()
	if f.Tool() != kubectlGetPodsTool {
		t.Fatalf("unexpected tool: %q", f.Tool())
	}
}

func TestGetPodsCompactionCases(t *testing.T) {
	f := NewKubectlGetPodsFilter()
	cases := []struct {
		name         string
		lines        []string
		wantContains []string
	}{
		{
			name: "compacts-healthy-keeps-anomaly",
			lines: []string{
				"NAME READY STATUS RESTARTS AGE",
				"api-1 1/1 Running 0 3m",
				"api-2 1/1 Running 0 2m",
				"api-3 0/1 CrashLoopBackOff 5 1m",
			},
			wantContains: []string{"CrashLoopBackOff", "[2] pods: 1/1 Running 0"},
		},
		{
			name: "all-namespaces-grouping",
			lines: []string{
				"NAMESPACE NAME READY STATUS RESTARTS AGE",
				"kube-system dns-1 1/1 Running 0 3m",
				"kube-system dns-2 1/1 Running 0 2m",
				"default app-1 1/1 Running 0 1m",
			},
			wantContains: []string{"kube-system: [2 pods: all Running]"},
		},
		{
			name: "all-namespaces-mixed-health-summary",
			lines: []string{
				"NAMESPACE NAME READY STATUS RESTARTS AGE",
				"ccp-bench healthy 1/1 Running 0 1m",
				"ccp-bench broken 0/1 ErrImagePull 0 1m",
			},
			wantContains: []string{"ccp-bench: [2 pods: 1 healthy, 1 unhealthy]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := decisionAtEOF(f, kubectlGetPodsTool, tc.lines...)
			for _, want := range tc.wantContains {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got %q", want, d.Output)
				}
			}
		})
	}
}

func TestGetPodsUnknownHeaderFallsBackToPassthrough(t *testing.T) {
	f := NewKubectlGetPodsFilter()
	d := decisionAtEOF(
		f,
		kubectlGetPodsTool,
		"NOM NOM",
		"pod-1 1/1 Running 0 1m",
	)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on unknown header fallback, got %q", d.Action)
	}
	if d.Output != "NOM NOM\npod-1 1/1 Running 0 1m\n" {
		t.Fatalf("expected passthrough output on unknown header, got %q", d.Output)
	}
}
