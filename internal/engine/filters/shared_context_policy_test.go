package filters

import (
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestSharedContextFiltersCollectOnEOFAndFlushOnExit(t *testing.T) {
	tests := []struct {
		name     string
		filter   engine.ToolFilter
		dispatch string
		line     string
	}{
		{
			name:   "gradle",
			filter: NewGradleFilter(),
			line:   "FAILURE: Build failed with an exception.\n",
		},
		{
			name:     "maven",
			filter:   NewMavenFilter(),
			dispatch: "maven|parallel=0",
			line:     "[ERROR] Failed to execute goal org.apache.maven.plugins:maven-compiler-plugin:3.11.0:compile\n",
		},
		{
			name:     "node",
			filter:   NewNodeFilter(),
			dispatch: "node|mode=runtime",
			line:     "Error: boom\n",
		},
		{
			name:     "deno",
			filter:   NewDenoFilter(),
			dispatch: "deno|mode=run",
			line:     "error: boom\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			mem.Add(tc.line, tc.line, 1)

			tool := tc.filter.Tool()
			eofDecision := tc.filter.Process(engine.Event{
				Type:     engine.EventEOF,
				Tool:     tool,
				Dispatch: tc.dispatch,
				Stream:   engine.StdoutStream,
			}, mem)
			if eofDecision.Action != engine.ActionCollect {
				t.Fatalf("expected EOF collect, got %q", eofDecision.Action)
			}

			exitDecision := tc.filter.Process(engine.Event{
				Type:     engine.EventExit,
				Tool:     tool,
				Dispatch: tc.dispatch,
				Stream:   engine.StdoutStream,
				ExitCode: 1,
			}, mem)
			if exitDecision.Action != engine.ActionFlush {
				t.Fatalf("expected Exit flush, got %q", exitDecision.Action)
			}
		})
	}
}
