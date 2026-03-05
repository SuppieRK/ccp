package engine

import "testing"

const (
	testLineA = "line-a\n"
)

func TestUnknownToolPassthrough(t *testing.T) {
	e := NewEngine(Config{NeverDropPatterns: DefaultNeverDropPatterns()})
	out := e.Process("stdout", "unknown", Input{Line: testLineA})
	if !out.Ready || out.Output != testLineA {
		t.Fatalf("unexpected passthrough output: %#v", out)
	}
}

func TestDedupeForRegisteredTool(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{collectingFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "sh", sharedContext: true},
		}},
	})
	if out := e.Process("stdout", "sh", Input{Line: "same\n"}); out.Ready {
		t.Fatalf("expected collect, got %#v", out)
	}
	if out := e.Process("stdout", "sh", Input{Line: "same\n"}); out.Ready {
		t.Fatalf("expected duplicate collect, got %#v", out)
	}
	if out := e.Process("stdout", "sh", Input{EOF: true}); out.Ready {
		t.Fatalf("expected shared context to wait for both EOFs, got %#v", out)
	}
	out := e.Process("stderr", "sh", Input{EOF: true})
	if !out.Ready || out.Output != "same\n" {
		t.Fatalf("unexpected flush output: %#v", out)
	}
}

func TestContextCleanupOnCompleteEOF(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{collectingFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "sh", sharedContext: true},
		}},
	})
	_ = e.Process("stdout", "sh", Input{Line: "a\n"})
	_ = e.Process("stderr", "sh", Input{Line: "b\n"})
	_ = e.Process("stdout", "sh", Input{EOF: true})
	if got := len(e.contexts); got != 1 {
		t.Fatalf("expected context to remain until all streams EOF, got %d", got)
	}
	_ = e.Process("stderr", "sh", Input{EOF: true})
	if got := len(e.contexts); got != 0 {
		t.Fatalf("expected context cleanup after complete EOF, got %d", got)
	}
}

func TestNoopFilterExists(t *testing.T) {
	f := NewNoopFilter("x")
	if f.Tool() != "x" {
		t.Fatalf("unexpected tool: %s", f.Tool())
	}
}

func TestExitEventDeliveredToFilter(t *testing.T) {
	seen := false
	code := 0
	f := decisionFilter{
		engineTestFilterBase: engineTestFilterBase{tool: "sh"},
		decide: func(ev Event) Decision {
			if ev.Type == EventExit {
				seen = true
				code = ev.ExitCode
				return Decision{Action: ActionIgnore}
			}
			if ev.Type == EventEOF {
				return Decision{Action: ActionFlush}
			}
			return Decision{Action: ActionCollect}
		},
	}
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters:           []ToolFilter{f},
	})
	_ = e.Process("stdout", "sh", Input{Exit: true, Code: 7})
	if !seen {
		t.Fatal("expected filter to receive exit event")
	}
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
}

func TestDisableAuditSkipsVerboseAuditFields(t *testing.T) {
	e := NewEngine(Config{
		NeverDropPatterns: DefaultNeverDropPatterns(),
		DisableAudit:      true,
	})
	out := e.Process("stdout", "unknown", Input{Line: testLineA})
	if out.Audit.Action != ActionImmediate {
		t.Fatalf("unexpected action: %q", out.Audit.Action)
	}
	if out.Audit.Raw != "" {
		t.Fatalf("expected raw audit field omitted, got %q", out.Audit.Raw)
	}
	if out.Audit.DerivedKey != "" || out.Audit.Mask != "" || out.Audit.Collision {
		t.Fatalf("expected verbose audit fields omitted, got %#v", out.Audit)
	}
}

type collectingFilter struct {
	engineTestFilterBase
}

func (c collectingFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	if ev.Type == EventEOF {
		return Decision{Action: ActionFlush}
	}
	return Decision{Action: ActionCollect}
}
