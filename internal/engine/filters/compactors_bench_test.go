package filters

import (
	"strconv"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
	kubectlfilters "go-command-compression-proxy/internal/engine/filters/kubectl"
)

func BenchmarkCompactFindOutput(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("./pkg/file")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(".go\n")
	}
	raw := sb.String()
	cfg := findDispatch{
		pattern:    "*.go",
		fileType:   "f",
		maxResults: 200,
		root:       ".",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compactFindOutput(raw, cfg)
	}
}

func BenchmarkCompactPNPMOutputList(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 300; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"name":"pkg`)
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(`","version":"1.0.0"}`)
	}
	sb.WriteString("]\n")
	raw := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compactPNPMOutput(raw, pnpmDispatch{mode: "list"}, 0)
	}
}

func BenchmarkCompactGrepOutput(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("pkg/file")
		sb.WriteString(strconv.Itoa(i % 20))
		sb.WriteString(".go:")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(": match line\n")
	}
	raw := sb.String()
	cfg := grepDispatch{maxResults: 120}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compactGrepOutput(raw, cfg)
	}
}

func BenchmarkKubectlTableCompactorPods(b *testing.B) {
	f := kubectlfilters.NewKubectlGetPodsFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("NAME READY STATUS RESTARTS AGE\n", "h", 1)
	for i := 0; i < 400; i++ {
		line := "pod-" + strconv.Itoa(i) + " 1/1 Running 0 1m\n"
		_ = mem.Add(line, strconv.Itoa(i), uint64(i+2))
	}
	ev := engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream, Tool: "kubectl get pods"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Process(ev, mem)
	}
}
