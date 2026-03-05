package kubectlfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestGetServicesMetadata(t *testing.T) {
	f := NewKubectlGetServicesFilter()
	if f.Tool() != "kubectl get services" {
		t.Fatalf("unexpected tool: %q", f.Tool())
	}
}

func TestGetServicesCompactionCases(t *testing.T) {
	f := NewKubectlGetServicesFilter()
	cases := []struct {
		name         string
		lines        []string
		wantContains []string
	}{
		{
			name: "compacts-by-signature",
			lines: []string{
				"NAME TYPE CLUSTER-IP EXTERNAL-IP PORT(S) AGE",
				"svc-a ClusterIP 10.0.0.1 <none> 80/TCP 3d",
				"svc-b ClusterIP 10.0.0.2 <none> 80/TCP 3d",
			},
			wantContains: []string{"[2] services: ClusterIP 80/TCP"},
		},
		{
			name: "all-namespaces-summary",
			lines: []string{
				"NAMESPACE NAME TYPE CLUSTER-IP EXTERNAL-IP PORT(S) AGE",
				"kube-system kube-dns ClusterIP 10.0.0.10 <none> 53/UDP,53/TCP 3d",
				"kube-system metrics ClusterIP 10.0.0.11 <none> 443/TCP 3d",
				"default app ClusterIP 10.0.0.12 <none> 80/TCP 1d",
			},
			wantContains: []string{"kube-system: [2 services: all Running]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := decisionAtEOF(f, "kubectl get services", tc.lines...)
			for _, want := range tc.wantContains {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got %q", want, d.Output)
				}
			}
		})
	}
}

func TestGetServicesUnknownHeaderFallsBackToPassthrough(t *testing.T) {
	f := NewKubectlGetServicesFilter()
	d := decisionAtEOF(
		f,
		"kubectl get services",
		"NOM NOM",
		"svc-a ClusterIP 10.0.0.1 <none> 80/TCP 3d",
	)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on unknown header fallback, got %q", d.Action)
	}
	if d.Output != "NOM NOM\nsvc-a ClusterIP 10.0.0.1 <none> 80/TCP 3d\n" {
		t.Fatalf("expected passthrough output on unknown header, got %q", d.Output)
	}
}
