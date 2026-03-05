package npxfilters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestNpxPrismaToolName(t *testing.T) {
	if NewNpxPrismaFilter().Tool() != "npx prisma" {
		t.Fatal("unexpected tool name")
	}
}

func TestNpxPrismaPreparePreservesArgs(t *testing.T) {
	f := NewNpxPrismaFilter()
	in := []string{"validate", "--schema", "prisma/schema.prisma"}
	prep := f.Prepare(in)
	if !slices.Equal(prep.NormalizedArgs, in) {
		t.Fatalf("expected args preserved, got %#v", prep.NormalizedArgs)
	}
}

func TestNpxPrismaSummarizeSuccess(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		want     string
		contains bool
	}{
		{
			name: "validate",
			raw:  "Prisma schema loaded from prisma/schema.prisma\nThe schema at prisma/schema.prisma is valid 🚀\n",
			want: "prisma validate: ok\n",
		},
		{
			name:     "format",
			raw:      "Prisma schema loaded from prisma/format.prisma\nFormatted prisma/format.prisma in 15ms 🚀\n",
			want:     "prisma format: ok prisma/format.prisma",
			contains: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ok := summarizePrismaSuccess(tt.raw)
			if !ok {
				t.Fatalf("expected summary, got ok=false out=%q", out)
			}
			if tt.contains {
				if !strings.Contains(out, tt.want) {
					t.Fatalf("expected output to contain %q, got %q", tt.want, out)
				}
				return
			}
			if out != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, out)
			}
		})
	}
}

func TestNpxPrismaFailurePassthrough(t *testing.T) {
	f := NewNpxPrismaFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := "Error: Prisma schema validation\nbad.prisma\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, ExitCode: 1}, mem)
	if d.Action != engine.ActionFlush || d.Output != raw {
		t.Fatalf("expected failure passthrough, got %#v", d)
	}
}

func TestNpxPrismaStderrImmediate(t *testing.T) {
	f := NewNpxPrismaFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "network error\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate || d.Output != "network error\n" {
		t.Fatalf("expected immediate stderr passthrough, got %#v", d)
	}
}

func TestNpxPrismaCollectsStdoutLineAndTick(t *testing.T) {
	f := NewNpxPrismaFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestNpxPrismaStdoutEOFIgnores(t *testing.T) {
	f := NewNpxPrismaFilter()
	d := f.Process(engine.Event{Type: engine.EventEOF, Stream: engine.StdoutStream}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore/no output on stdout EOF, got %#v", d)
	}
}

func TestNpxPrismaWrapperOnlyOutputIgnores(t *testing.T) {
	f := NewNpxPrismaFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"Need to install the following packages:",
		"  prisma@6.0.0",
		"Ok to proceed? (y)",
		"npm WARN exec The following package was not found and will be installed",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, ExitCode: 0}, mem)
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore for wrapper-only output, got %#v", d)
	}
}

func TestNpxPrismaUnknownSuccessFallbackFlushesStrippedRaw(t *testing.T) {
	f := NewNpxPrismaFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := strings.Join([]string{
		"Need to install the following packages:",
		"  prisma@6.0.0",
		"Ok to proceed? (y)",
		"Prisma schema loaded from prisma/schema.prisma",
		"Some non-summarized success line",
	}, "\n") + "\n"
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Stream: engine.StdoutStream, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush fallback for unknown success output, got %#v", d)
	}
	want := "Prisma schema loaded from prisma/schema.prisma\nSome non-summarized success line\n"
	if d.Output != want {
		t.Fatalf("expected stripped raw fallback output, got %q", d.Output)
	}
}
