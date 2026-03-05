package runner

import (
	"path/filepath"
	"strings"
	"testing"
)

func benchmarkRunnerWithCapture(b *testing.B) *Runner {
	b.Helper()
	dir := b.TempDir()
	r := &Runner{
		opts: Options{
			CaptureRaw: true,
		},
		capture: &rawCapture{
			stdoutPath: filepath.Join(dir, "stdout.txt"),
			stderrPath: filepath.Join(dir, "stderr.txt"),
		},
	}
	b.Cleanup(func() { r.closeCapture() })
	return r
}

func BenchmarkRunnerCaptureRawLineInterleave(b *testing.B) {
	r := benchmarkRunnerWithCapture(b)
	line := "interleaved-line\n"
	b.SetBytes(int64(len(line) * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.captureRawLine("stdout", line)
		r.captureRawLine("stderr", line)
	}
}

func BenchmarkRunnerCaptureRawLineLarge(b *testing.B) {
	r := benchmarkRunnerWithCapture(b)
	line := strings.Repeat("x", 64*1024) + "\n"
	b.SetBytes(int64(len(line)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.captureRawLine("stdout", line)
	}
}
