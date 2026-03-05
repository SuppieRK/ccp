package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const readFixtureErrFmt = "read fixture: %v"

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name)
}

func newFixtureEngine(commandID string) *Engine {
	e := NewEngine(Config{
		NeverDropPatterns: nil,
		Filters: []ToolFilter{fixtureCollectFilter{
			engineTestFilterBase: engineTestFilterBase{tool: "fixture", sharedContext: true},
		}},
	})
	e.SetCommandID(commandID)
	return e
}

func TestCanonicalFixtureCorpusScaffolded(t *testing.T) {
	cases := []string{
		"noise-bomb.log",
		"race-condition.log",
		"deep-json.log",
		"collision-sensitive.log",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(name)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("missing canonical fixture %s: %v", path, err)
			}
		})
	}
}

func TestDeterminismReplayRaceConditionFixture(t *testing.T) {
	path := fixturePath("race-condition.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(readFixtureErrFmt, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := runFixtureReplay(lines)
	for i := 0; i < 100; i++ {
		got := runFixtureReplay(lines)
		if got != want {
			t.Fatalf("determinism mismatch at run %d\nwant=%q\ngot=%q", i, want, got)
		}
	}
}

func TestCollisionSafetyFixtureAndPoisonCase(t *testing.T) {
	e := newFixtureEngine("fixture")

	path := fixturePath("collision-sensitive.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(readFixtureErrFmt, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		out := e.Process(string(StdoutStream), "fixture", Input{Line: line + "\n"})
		if out.Audit.Collision {
			t.Fatalf("unexpected critical collision for line %q", line)
		}
	}

	e = newFixtureEngine("poison")
	first := e.Process(string(StdoutStream), "fixture", Input{Line: "trace=1234567890\n"})
	second := e.Process(string(StdoutStream), "fixture", Input{Line: "trace=2234567890\n"})
	if first.Audit.Collision {
		t.Fatal("first line should not collide")
	}
	if !second.Audit.Collision {
		t.Fatal("expected poison-case collision to be reported")
	}
}

func TestLatencyGateUnderTwoMillisecondsPerLine(t *testing.T) {
	path := fixturePath("noise-bomb.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(readFixtureErrFmt, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("fixture has no lines")
	}

	e := newFixtureEngine("latency-gate")
	start := time.Now()
	for _, line := range lines {
		_ = e.Process(string(StdoutStream), "fixture", Input{Line: line + "\n"})
	}
	_ = e.Process(string(StdoutStream), "fixture", Input{EOF: true})
	elapsed := time.Since(start)

	avg := elapsed / time.Duration(len(lines))
	if avg > 2*time.Millisecond {
		t.Fatalf("latency gate failed: avg %s per line exceeds 2ms", avg)
	}
}

func runFixtureReplay(lines []string) string {
	e := newFixtureEngine("determinism")
	var b strings.Builder
	for _, line := range lines {
		stream := StdoutStream
		payload := line
		if strings.HasPrefix(line, "[stderr] ") {
			stream = StderrStream
			payload = strings.TrimPrefix(line, "[stderr] ")
		}
		if strings.HasPrefix(line, "[stdout] ") {
			payload = strings.TrimPrefix(line, "[stdout] ")
		}
		_ = e.Process(string(stream), "fixture", Input{Line: payload + "\n"})
	}
	if out := e.Process(string(StdoutStream), "fixture", Input{EOF: true}); out.Output != "" {
		b.WriteString(out.Output)
	}
	if out := e.Process(string(StderrStream), "fixture", Input{EOF: true}); out.Output != "" {
		b.WriteString(out.Output)
	}
	return b.String()
}

type fixtureCollectFilter struct {
	engineTestFilterBase
}

func (f fixtureCollectFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	if ev.Type == EventEOF {
		return Decision{Action: ActionFlush}
	}
	return Decision{Action: ActionCollect}
}
