package kubectlfilters

import "go-command-compression-proxy/internal/engine"

func decisionAtEOF(f engine.ToolFilter, dispatch string, lines ...string) engine.Decision {
	mem := engine.NewOrderedSetBuffer()
	for i, line := range lines {
		_ = mem.Add(line+"\n", line, uint64(i+1))
	}
	return f.Process(engine.Event{Type: engine.EventEOF, Tool: "kubectl", Dispatch: dispatch, Stream: engine.StdoutStream}, mem)
}
