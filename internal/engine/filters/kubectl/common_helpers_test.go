package kubectlfilters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const kubectlGetPodsDispatch = "kubectl get pods"

func TestKubectlCommonHelperPaths(t *testing.T) {
	t.Parallel()

	if !allowlistedGetArgs([]string{"-A", "-o", "wide"}) {
		t.Fatal("expected allowlisted args to pass")
	}
	if allowlistedGetArgs([]string{"--output", "json"}) {
		t.Fatal("expected structured output args to fail allowlist")
	}
	if !hasOutputValue([]string{"--output=wide"}) {
		t.Fatal("expected output value detection")
	}
	if got := outputValue([]string{"-o", "wide"}); got != "wide" {
		t.Fatalf("outputValue() = %q, want wide", got)
	}

	podsRow := tableRow{
		headerIndex: map[string]int{"STATUS": 0, "READY": 1, "RESTARTS": 2},
		fields:      []string{"Running", "1/1", "0"},
	}
	if healthy, _ := isPodsHealthy(podsRow); !healthy {
		t.Fatal("expected healthy pod")
	}
	nodesRow := tableRow{
		headerIndex: map[string]int{"STATUS": 0},
		fields:      []string{"Ready"},
	}
	if healthy, _ := isNodesHealthy(nodesRow); !healthy {
		t.Fatal("expected healthy node")
	}
	if healthy, _ := isServicesHealthy(); !healthy {
		t.Fatal("expected services to be treated as healthy")
	}
}

func TestKubectlLogsMethodCoverage(t *testing.T) {
	t.Parallel()
	f := NewKubectlLogsFilter()
	if f.Tool() != "kubectl logs" {
		t.Fatalf("Tool() = %q", f.Tool())
	}
	if got := f.Aliases(); got != nil {
		t.Fatalf("Aliases() = %v, want nil", got)
	}
	if f.ContextKey(engine.Event{CommandID: "c", Tool: "kubectl logs", Stream: engine.StdoutStream}) == "" {
		t.Fatal("ContextKey() returned empty key")
	}
	if got := f.MaskingHorizon(); got != 4096 {
		t.Fatalf("MaskingHorizon() = %d, want 4096", got)
	}
}

func TestKubectlTableCompactorMethodCoverage(t *testing.T) {
	t.Parallel()

	f := NewKubectlGetPodsFilter()
	if got := f.Tool(); got != kubectlGetPodsDispatch {
		t.Fatalf("Tool() = %q, want kubectl get pods", got)
	}
	if got := f.Aliases(); got != nil {
		t.Fatalf("Aliases() = %v, want nil", got)
	}
	if got := f.MaskingHorizon(); got != 0 {
		t.Fatalf("MaskingHorizon() = %d, want 0", got)
	}

	prep := f.Prepare([]string{"--output", "json"})
	if !prep.ForcePassthrough {
		t.Fatalf("expected structured output prepare to force passthrough, got %#v", prep)
	}
	if f.ContextKey(engine.Event{CommandID: "c", Tool: kubectlGetPodsDispatch, Stream: engine.StdoutStream}) == "" {
		t.Fatal("ContextKey() returned empty key")
	}

	d := f.Process(engine.Event{
		Type:   engine.EventLine,
		Tool:   kubectlGetPodsDispatch,
		Stream: engine.StderrStream,
		Line:   "No resources found in default namespace.\n",
	}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "No pods found\n" {
		t.Fatalf("stderr no-resources decision = %#v", d)
	}
}
