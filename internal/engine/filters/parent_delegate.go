package filters

import "go-command-compression-proxy/internal/engine"

func newSubcommandRegistry(filters ...engine.ToolFilter) (*engine.ToolFilterRegistry, error) {
	reg := engine.NewToolFilterRegistry()
	for _, f := range filters {
		if err := reg.Register(f); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func prepareParentInitError(args []string, tool string, initErr error) (engine.PrepareResult, bool) {
	if initErr == nil {
		return engine.PrepareResult{}, false
	}
	return engine.PrepareResult{
		NormalizedArgs:   append([]string{}, args...),
		ForcePassthrough: true,
		Ambiguous:        true,
		Reason:           tool + " filter unavailable: " + initErr.Error(),
	}, true
}

// applyDelegatedPrepare normalizes delegated prepare results back into parent args.
func applyDelegatedPrepare(
	args []string,
	consumed int,
	prep engine.PrepareResult,
	dispatchKey string,
	clearDispatchOnForce bool,
) engine.PrepareResult {
	normalized := prep.NormalizedArgs
	if normalized == nil {
		normalized = args[consumed:]
	}
	prep.NormalizedArgs = append(append([]string{}, args[:consumed]...), normalized...)
	prep.DispatchKey = dispatchKey
	if clearDispatchOnForce && prep.ForcePassthrough {
		prep.DispatchKey = ""
	}
	return prep
}

func delegatedContextKey(
	tool string,
	ev engine.Event,
	resolve func(engine.Event) engine.ToolFilter,
) string {
	f := resolve(ev)
	if f != nil {
		return f.ContextKey(ev)
	}
	if ev.Tool == "" {
		ev.Tool = tool
	}
	return engine.StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}

func delegatedProcess(ev engine.Event, mem *engine.OrderedSetBuffer, resolve func(engine.Event) engine.ToolFilter) engine.Decision {
	f := resolve(ev)
	if f != nil {
		return f.Process(ev, mem)
	}
	if ev.Type == engine.EventLine {
		return engine.Decision{Action: engine.ActionImmediate, Output: ev.Line}
	}
	return engine.Decision{Action: engine.ActionIgnore}
}

func stderrImmediateOrIgnore(
	ev engine.Event,
	normalize func(string) string,
) (engine.Decision, bool) {
	if ev.Stream != engine.StderrStream {
		return engine.Decision{}, false
	}
	if ev.Type != engine.EventLine {
		return engine.Decision{Action: engine.ActionIgnore}, true
	}
	out := ev.Line
	if normalize != nil {
		out = normalize(ev.Line)
	}
	return engine.Decision{Action: engine.ActionImmediate, Output: out}, true
}

func collectOnLineTickEOF(ev engine.Event) (engine.Decision, bool) {
	if ev.Type == engine.EventLine || ev.Type == engine.EventTick || ev.Type == engine.EventEOF {
		return engine.Decision{Action: engine.ActionCollect}, true
	}
	return engine.Decision{}, false
}
