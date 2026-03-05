package engine

type noopFilter struct {
	tool string
}

// NewNoopFilter returns a passthrough filter for unknown tools.
func NewNoopFilter(tool string) ToolFilter {
	return noopFilter{tool: tool}
}

func (n noopFilter) Tool() string      { return n.tool }
func (n noopFilter) Aliases() []string { return nil }

func (n noopFilter) Prepare(args []string) PrepareResult {
	return PrepareResult{NormalizedArgs: args}
}

func (n noopFilter) ContextKey(ev Event) string {
	return StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func (n noopFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	if ev.Type == EventLine {
		return Decision{Action: ActionImmediate, Output: ev.Line}
	}
	return Decision{Action: ActionIgnore}
}
func (n noopFilter) MaskingHorizon() int { return 0 }
