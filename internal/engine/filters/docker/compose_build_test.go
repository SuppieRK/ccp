package dockerfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestComposeBuildCompactionSummarizesServices(t *testing.T) {
	raw := strings.Join([]string{
		"project-api  Built",
		"project-web  Built",
		"#2 [web internal] load build definition from Dockerfile           0.0s",
		"#8 [api 1/4] FROM docker.io/library/golang:1.26-alpine           0.0s",
		"#9 [web 1/4] FROM docker.io/library/node:20-alpine               0.0s",
		"#18 [api] exporting to image                                     0.0s",
		"#19 [web] exporting to image                                     0.0s",
	}, "\n") + "\n"

	out, ok := compactComposeBuild(raw, 15)
	if !ok {
		t.Fatal("expected compose build compaction")
	}
	if strings.Contains(out, "[+] Building") {
		t.Fatalf("did not expect synthetic summary line, got:\n%s", out)
	}
	if !strings.Contains(out, "[ok] api built") || !strings.Contains(out, "[ok] web built") {
		t.Fatalf("expected per-service build rows, got:\n%s", out)
	}
}

func TestComposeBuildLowConfidenceFallback(t *testing.T) {
	raw := "random build output without buildkit markers\n"
	if _, ok := compactComposeBuild(raw, 15); ok {
		t.Fatalf("expected low-confidence fallback")
	}
}

func TestComposeBuildFailureFallback(t *testing.T) {
	raw := strings.Join([]string{
		"[+] Building 2.0s (4/4)",
		" => [web 1/2] RUN npm install",
		"ERROR: failed to solve: process \"/bin/sh -c npm install\" did not complete successfully",
	}, "\n") + "\n"
	if _, ok := compactComposeBuild(raw, 15); ok {
		t.Fatalf("expected failure output fallback")
	}
}

func TestComposeBuildProcessRuntimeCollectionAndFlushParity(t *testing.T) {
	f := NewComposeBuildFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "build error\n"}, mem); got.Action != engine.ActionImmediate {
		t.Fatalf("expected stderr immediate, got %#v", got)
	}
}
