package engine

import (
	"strconv"
	"strings"
	"testing"
)

type decisionFilter struct {
	engineTestFilterBase
	decide func(ev Event) Decision
}

func (f decisionFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	if f.decide == nil {
		return Decision{Action: ActionCollect}
	}
	return f.decide(ev)
}

func TestImmediateDecisionReturnsOutput(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{decisionFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "imm"},
			decide: func(ev Event) Decision {
				if ev.Type == EventLine {
					return Decision{Action: ActionImmediate, Output: "immediate\n"}
				}
				return Decision{Action: ActionCollect}
			},
		}},
	})
	out := e.Process(string(StdoutStream), "imm", Input{Line: "a\n"})
	if !out.Ready || out.Output != "immediate\n" {
		t.Fatalf("unexpected immediate output: %#v", out)
	}
}

func TestIgnoreOnExitCleansContext(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{decisionFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "ign"},
			decide: func(ev Event) Decision {
				if ev.Type == EventExit {
					return Decision{Action: ActionIgnore}
				}
				return Decision{Action: ActionCollect}
			},
		}},
	})
	_ = e.Process(string(StdoutStream), "ign", Input{Line: "buffered\n"})
	if got := len(e.contexts); got != 1 {
		t.Fatalf("expected context before exit, got %d", got)
	}
	out := e.Process(string(StdoutStream), "ign", Input{Exit: true, Code: 3})
	if out.Ready {
		t.Fatalf("expected ignore decision to keep output empty, got %#v", out)
	}
	if got := len(e.contexts); got != 0 {
		t.Fatalf("expected context cleanup on exit ignore, got %d", got)
	}
}

func TestProcessTickFlushesBufferedOutput(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{decisionFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "tick"},
			decide: func(ev Event) Decision {
				if ev.Type == EventTick {
					return Decision{Action: ActionFlush}
				}
				return Decision{Action: ActionCollect}
			},
		}},
	})
	_ = e.Process(string(StdoutStream), "tick", Input{Line: "tick-buffered\n"})
	outs := e.ProcessTick("tick")
	if len(outs) != 1 {
		t.Fatalf("expected one tick output, got %d", len(outs))
	}
	if !outs[0].Ready || !strings.Contains(outs[0].Output, "tick-buffered") {
		t.Fatalf("unexpected tick flush output: %#v", outs[0])
	}
}

func TestNeverDropPatternsRetainDuplicateCriticalLines(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: DefaultNeverDropPatterns(),
		Filters: []ToolFilter{collectingFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "sh", sharedContext: true},
		}},
	})
	_ = e.Process(string(StdoutStream), "sh", Input{Line: "error: boom\n"})
	_ = e.Process(string(StdoutStream), "sh", Input{Line: "error: boom\n"})
	_ = e.Process(string(StdoutStream), "sh", Input{EOF: true})
	out := e.Process(string(StderrStream), "sh", Input{EOF: true})
	if !out.Ready {
		t.Fatalf("expected flush on complete EOF, got %#v", out)
	}
	if got := strings.Count(out.Output, "error: boom"); got != 2 {
		t.Fatalf("expected duplicate critical lines retained, got %d in %q", got, out.Output)
	}
}

func TestFlushOnExitWithoutSeenStreamsCleansContext(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{decisionFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "fx"},
			decide: func(ev Event) Decision {
				if ev.Type == EventExit {
					return Decision{Action: ActionFlush}
				}
				return Decision{Action: ActionCollect}
			},
		}},
	})
	_ = e.Process(string(StdoutStream), "fx", Input{Exit: true, Code: 0})
	if got := len(e.contexts); got != 0 {
		t.Fatalf("expected context cleanup for empty-stream flush exit, got %d", got)
	}
}

func TestHighWaterFlushIncludesElisionMarker(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{collectingFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "hw"},
		}},
	})
	e.SetCommandID("high-water")

	var out Output
	for i := 0; i <= maxBufferedLines; i++ {
		out = e.Process(string(StdoutStream), "hw", Input{Line: "line-" + strconv.Itoa(i) + "\n"})
	}

	if !out.Ready {
		t.Fatalf("expected forced flush at high-water breach, got %#v", out)
	}
	if !strings.Contains(out.Output, "context limit reached; 1 lines elided") {
		t.Fatalf("expected high-water elision marker, got %q", out.Output)
	}
}

func TestAuditRecordContainsAnalyzabilityMetadata(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{collectingFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "audit", sharedContext: true},
		}},
	})
	e.SetCommandID("cmd-123")

	out := e.Process(string(StdoutStream), "audit", Input{Line: "trace=1234567890\n"})
	if out.Audit.CommandID != "cmd-123" {
		t.Fatalf("expected command id, got %q", out.Audit.CommandID)
	}
	if out.Audit.Tool != "audit" {
		t.Fatalf("expected tool, got %q", out.Audit.Tool)
	}
	if out.Audit.Stream != StdoutStream {
		t.Fatalf("expected stream stdout, got %q", out.Audit.Stream)
	}
	if out.Audit.Action != ActionCollect {
		t.Fatalf("expected action collect, got %q", out.Audit.Action)
	}
	if out.Audit.DerivedKey == "" {
		t.Fatal("expected derived key to be present")
	}
	if out.Audit.Mask == "" {
		t.Fatal("expected mask metadata to be present")
	}
}
