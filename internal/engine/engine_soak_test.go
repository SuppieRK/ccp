package engine

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type soakSharedFilter struct {
	engineTestFilterBase
}

func (f soakSharedFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	switch ev.Type {
	case EventLine, EventTick:
		return Decision{Action: ActionCollect}
	case EventEOF:
		return Decision{Action: ActionFlush}
	case EventExit:
		return Decision{Action: ActionIgnore}
	default:
		return Decision{Action: ActionCollect}
	}
}

func newSoakSharedEngine() *Engine {
	return NewEngine(Config{
		Filters: []ToolFilter{
			soakSharedFilter{engineTestFilterBase: engineTestFilterBase{tool: "soak", sharedContext: true}},
		},
	})
}

func assertNoActiveSoakContexts(t *testing.T, e *Engine, label string) {
	t.Helper()
	if got := len(e.contexts); got != 0 {
		t.Fatalf("%s: expected no active contexts, got %d", label, got)
	}
}

func TestSoakSharedContextPartialTerminationCleansUp(t *testing.T) {
	e := newSoakSharedEngine()

	for i := 0; i < 500; i++ {
		e.SetCommandID(fmt.Sprintf("soak-%d", i))
		_ = e.Process("stdout", "soak", Input{Line: fmt.Sprintf("out-%d\n", i)})
		_ = e.Process("stderr", "soak", Input{Line: fmt.Sprintf("err-%d\n", i)})

		eofOut := e.Process("stdout", "soak", Input{EOF: true})
		if eofOut.Ready {
			t.Fatalf("iteration %d: expected first EOF to stay buffered in shared context", i)
		}

		_ = e.ProcessTick("soak")
		_ = e.Process("stderr", "soak", Input{Line: fmt.Sprintf("tail-%d\n", i)})
		_ = e.Process("stderr", "soak", Input{Exit: true, Code: 0})

		assertNoActiveSoakContexts(t, e, fmt.Sprintf("iteration %d cleanup on exit", i))
	}
}

func TestSoakSharedContextFlushConsistencyOnDualEOF(t *testing.T) {
	e := newSoakSharedEngine()

	for i := 0; i < 250; i++ {
		outLine := fmt.Sprintf("out-%d\n", i)
		errLine := fmt.Sprintf("err-%d\n", i)
		_ = e.Process("stdout", "soak", Input{Line: outLine})
		_ = e.Process("stderr", "soak", Input{Line: errLine})

		firstEOF := e.Process("stdout", "soak", Input{EOF: true})
		if firstEOF.Ready {
			t.Fatalf("iteration %d: expected first EOF to be deferred", i)
		}

		secondEOF := e.Process("stderr", "soak", Input{EOF: true})
		if !secondEOF.Ready {
			t.Fatalf("iteration %d: expected second EOF flush", i)
		}
		if !strings.Contains(secondEOF.Output, strings.TrimSpace(outLine)) || !strings.Contains(secondEOF.Output, strings.TrimSpace(errLine)) {
			t.Fatalf("iteration %d: expected mixed-stream flush to include both lines, got %q", i, secondEOF.Output)
		}
		assertNoActiveSoakContexts(t, e, fmt.Sprintf("iteration %d cleanup after dual EOF", i))
	}
}

func TestConcurrentProcessAndTickMaintainsContextInvariants(t *testing.T) {
	e := newSoakSharedEngine()

	var tickWG sync.WaitGroup
	stopTicks := make(chan struct{})
	tickWG.Add(1)
	go func() {
		defer tickWG.Done()
		for {
			select {
			case <-stopTicks:
				return
			default:
				_ = e.ProcessTick("soak")
			}
		}
	}()

	workers := 4
	var workerWG sync.WaitGroup
	workerWG.Add(workers)
	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer workerWG.Done()
			for i := 0; i < 200; i++ {
				_ = e.Process("stdout", "soak", Input{Line: fmt.Sprintf("w%d-out-%d\n", w, i)})
				_ = e.Process("stderr", "soak", Input{Line: fmt.Sprintf("w%d-err-%d\n", w, i)})
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		workerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent process/tick run timed out")
	}
	close(stopTicks)
	tickWG.Wait()

	// Complete and clean context explicitly.
	_ = e.Process("stdout", "soak", Input{EOF: true})
	_ = e.Process("stderr", "soak", Input{EOF: true})
	_ = e.Process("stdout", "soak", Input{Exit: true, Code: 0})
	_ = e.Process("stderr", "soak", Input{Exit: true, Code: 0})

	assertNoActiveSoakContexts(t, e, "after concurrent run")
}
