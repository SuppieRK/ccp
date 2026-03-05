package filters

import (
	"strings"

	"go-command-compression-proxy/internal/engine"
)

// NewPythonFilter returns Python parent routing filter.
func NewPythonFilter() engine.ToolFilter {
	return &pythonFilter{pytest: NewPytestFilter()}
}

type pythonFilter struct {
	pytest engine.ToolFilter
}

func (p *pythonFilter) Tool() string { return "python" }

func (p *pythonFilter) Aliases() []string {
	return []string{
		"python3",
		"python.exe", "./python.exe",
		"python3.exe", "./python3.exe",
		"python.cmd", "./python.cmd",
		"python3.cmd", "./python3.cmd",
	}
}

func (p *pythonFilter) Prepare(args []string) engine.PrepareResult {
	prep := engine.PrepareResult{
		NormalizedArgs:   append([]string{}, args...),
		ForcePassthrough: true,
	}
	// REPL and interactive execution are explicit ambiguity boundaries.
	interactive := len(args) == 0
	moduleIdx := -1
	module := ""
	for i, arg := range args {
		trimmed := strings.TrimSpace(strings.ToLower(arg))
		if trimmed == "-i" || trimmed == "--interactive" {
			interactive = true
		}
		if trimmed != "-m" || i+1 >= len(args) {
			continue
		}
		moduleIdx = i
		module = strings.TrimSpace(args[i+1])
		break
	}
	if interactive {
		prep.Ambiguous = true
		prep.Reason = "interactive python invocation"
		return prep
	}
	// Non-pytest module invocation remains passthrough by design.
	if moduleIdx >= 0 && strings.EqualFold(module, "pytest") && p.pytest != nil {
		pytestPrep := p.pytest.Prepare(args[moduleIdx+2:])
		return applyDelegatedPrepare(args, moduleIdx+2, pytestPrep, p.pytest.Tool(), false)
	}
	return prep
}

func (p *pythonFilter) ContextKey(ev engine.Event) string {
	return delegatedContextKey(p.Tool(), ev, p.resolve)
}

func (p *pythonFilter) Process(ev engine.Event, mem *engine.OrderedSetBuffer) engine.Decision {
	return delegatedProcess(ev, mem, p.resolve)
}

func (p *pythonFilter) MaskingHorizon() int { return 0 }

func (p *pythonFilter) resolve(ev engine.Event) engine.ToolFilter {
	if strings.EqualFold(strings.TrimSpace(ev.Dispatch), "pytest") {
		return p.pytest
	}
	return nil
}
