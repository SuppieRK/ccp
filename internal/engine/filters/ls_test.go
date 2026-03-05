package filters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const expectedLSOutputAtEOF = "expected ls compactor to produce output at EOF"

func TestCompactLSBasicBehavior(t *testing.T) {
	input := "total 48\n" +
		"drwxr-xr-x  2 user  staff    64 Jan  1 12:00 .\n" +
		"drwxr-xr-x  2 user  staff    64 Jan  1 12:00 ..\n" +
		"drwxr-xr-x  2 user  staff    64 Jan  1 12:00 src\n" +
		"-rw-r--r--  1 user  staff  1234 Jan  1 12:00 Cargo.toml\n" +
		"-rw-r--r--  1 user  staff  5678 Jan  1 12:00 README.md\n" +
		"-rw-r--r--  1 user  staff   100 Jan  1 12:00 my file.txt\n" +
		"lrwxr-xr-x  1 user  staff    10 Jan  1 12:00 link -> target\n"

	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(input, input, 1)
	out := NewLSCompactor().Process(engine.Event{Type: engine.EventEOF, Tool: "ls"}, mem)
	assertLSFlushAtEOF(t, out)
	if !strings.Contains(out.Output, "src/") {
		t.Fatal("expected src directory")
	}
	if !strings.Contains(out.Output, "Cargo.toml  1.2K") {
		t.Fatal("expected compact size for Cargo.toml")
	}
	if !strings.Contains(out.Output, "README.md  5.5K") {
		t.Fatal("expected compact size for README.md")
	}
	if !strings.Contains(out.Output, "my file.txt  100B") {
		t.Fatal("expected filename with spaces")
	}
	if !strings.Contains(out.Output, "link -> target  10B") {
		t.Fatal("expected symlink entry")
	}
	if !strings.Contains(out.Output, "summary: 4 files, 1 dirs") {
		t.Fatal("expected plain-text summary")
	}
	if strings.Contains(out.Output, "📊") {
		t.Fatal("summary must be plain text only")
	}
}

func TestLSPrepareOnlyLongFlagsEnableNormalization(t *testing.T) {
	f := NewLSCompactor()

	cases := map[string]struct {
		args        []string
		passthrough bool
		normalized  []string
	}{
		"default passthrough":        {args: []string{}, passthrough: true},
		"plain path passthrough":     {args: []string{"src"}, passthrough: true},
		"show all passthrough":       {args: []string{"-a"}, passthrough: true},
		"human passthrough":          {args: []string{"-h"}, passthrough: true},
		"recursive passthrough":      {args: []string{"-R", "src"}, passthrough: true},
		"long normalizes":            {args: []string{"-l"}, normalized: []string{"-la", "."}},
		"long human normalizes":      {args: []string{"-lh", "src"}, normalized: []string{"-la", "src"}},
		"long all human normalizes":  {args: []string{"-lah", "src"}, normalized: []string{"-la", "src"}},
		"long with extra flag keeps": {args: []string{"-lR", "src"}, normalized: []string{"-la", "-R", "src"}},
		"long with --all strips all": {args: []string{"-l", "--all", "src"}, normalized: []string{"-la", "src"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough != tc.passthrough {
				t.Fatalf("args=%#v passthrough mismatch: got %v want %v", tc.args, prep.ForcePassthrough, tc.passthrough)
			}
			if tc.passthrough {
				return
			}
			if strings.Join(prep.NormalizedArgs, "||") != strings.Join(tc.normalized, "||") {
				t.Fatalf("args=%#v normalized mismatch: got %#v want %#v", tc.args, prep.NormalizedArgs, tc.normalized)
			}
		})
	}
}

func TestCompactLSEmptyHeaders(t *testing.T) {
	for _, header := range []string{"total 0\n", "insgesamt 0\n"} {
		t.Run(strings.TrimSpace(header), func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			_ = mem.Add(header, header, 1)
			out := NewLSCompactor().Process(engine.Event{Type: engine.EventEOF, Tool: "ls"}, mem)
			assertLSFlushAtEOF(t, out)
			if out.Output != "(empty)\n" {
				t.Fatalf("unexpected empty output for header %q: %q", strings.TrimSpace(header), out.Output)
			}
		})
	}
}

func TestLSProcessStderrImmediate(t *testing.T) {
	f := NewLSCompactor()
	d := f.Process(
		engine.Event{Type: engine.EventLine, Tool: "ls", Stream: engine.StderrStream, Line: "ls: cannot access missing\n"},
		engine.NewOrderedSetBuffer(),
	)
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr emission, got %q", d.Action)
	}
	if d.Output != "ls: cannot access missing\n" {
		t.Fatalf("unexpected stderr output: %q", d.Output)
	}
}

func TestLSShortListingEnrichmentFromFS(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "dir-a"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	d := runLSEOFShortListing("ls " + tmp)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on enriched short listing, got %q", d.Action)
	}
	if !strings.Contains(d.Output, "dir-a/") {
		t.Fatalf("expected enriched dir entry, got %q", d.Output)
	}
	if !strings.Contains(d.Output, "a.txt  3B") {
		t.Fatalf("expected enriched file size entry, got %q", d.Output)
	}
}

func TestLSShortListingFallbackWithoutFabrication(t *testing.T) {
	d := runLSEOFShortListing("ls /definitely/missing/for/ccp")
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush on raw short-listing fallback, got %q", d.Action)
	}
	if d.Output != "a.txt dir-a\n" {
		t.Fatalf("expected raw short-listing output without fabricated metadata, got %q", d.Output)
	}
}

func TestLSTinyOutputOmitsSummary(t *testing.T) {
	input := "total 8\n" +
		"drwxr-xr-x  2 user  staff   64 Jan  1 12:00 src\n" +
		"-rw-r--r--  1 user  staff  100 Jan  1 12:00 README.md\n"

	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add(input, input, 1)
	out := NewLSCompactor().Process(engine.Event{Type: engine.EventEOF, Tool: "ls"}, mem)
	assertLSFlushAtEOF(t, out)
	if strings.Contains(out.Output, "summary:") {
		t.Fatalf("did not expect summary for tiny output, got %q", out.Output)
	}
}

func assertLSFlushAtEOF(t *testing.T, out engine.Decision) {
	t.Helper()
	if out.Action != engine.ActionFlush {
		t.Fatal(expectedLSOutputAtEOF)
	}
}

func runLSEOFShortListing(dispatch string) engine.Decision {
	mem := engine.NewOrderedSetBuffer()
	raw := "a.txt\ndir-a\n"
	_ = mem.Add(raw, raw, 1)
	return NewLSCompactor().Process(engine.Event{
		Type:     engine.EventEOF,
		Tool:     "ls",
		Stream:   engine.StdoutStream,
		Dispatch: dispatch,
	}, mem)
}
