package engine

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const maxBufferedLines = 5000
const maxRawByKeyEntries = 4096
const maxActiveContexts = 2048

// Input describes one stream event delivered to the semantic engine.
type Input struct {
	Line     string
	Dispatch string
	EOF      bool
	Tick     bool
	Exit     bool
	Code     int
}

// Output is the engine decision materialized for the runner.
type Output struct {
	Ready   bool
	Output  string
	Collect bool
	Stream  StreamType
	Audit   AuditRecord
}

// Config controls engine behavior and startup filter registration.
type Config struct {
	NeverDropPatterns []string
	Registry          *ToolFilterRegistry
	Filters           []ToolFilter
	DisableAudit      bool
}

type contextState struct {
	mu         sync.Mutex
	buffer     *OrderedSetBuffer
	seenMask   uint8
	eofMask    uint8
	rawByKey   map[string]string
	rawKeyFull bool
	lastSeq    uint64
	stream     StreamType
	tool       string
	dispatch   string
	shared     bool
}

func newContextState(ev Event, shared bool) *contextState {
	ctx := &contextState{
		buffer:   NewOrderedSetBuffer(),
		rawByKey: map[string]string{},
		stream:   ev.Stream,
		tool:     ev.Tool,
		dispatch: ev.Dispatch,
		shared:   shared,
	}
	if shared {
		ctx.seenMask = streamMask(StdoutStream) | streamMask(StderrStream)
	}
	return ctx
}

func (c *contextState) applyEventMeta(ev Event) {
	c.lastSeq = ev.Sequence
	if ev.Type == EventLine {
		c.stream = ev.Stream
	}
	c.tool = ev.Tool
	if ev.Dispatch != "" {
		c.dispatch = ev.Dispatch
	}
}

// Engine performs stream-aware semantic compaction using registered tool filters.
type Engine struct {
	registry          *ToolFilterRegistry
	contexts          map[string]*contextState
	neverDropPatterns []string
	auditDisabled     bool
	mu                sync.Mutex
	sequence          uint64
	commandID         string
}

// NewEngine builds an engine with the provided registry/configured filters.
func NewEngine(cfg Config) *Engine {
	registry := cfg.Registry
	if registry == nil {
		registry = NewToolFilterRegistry()
	}
	for _, f := range cfg.Filters {
		if err := registry.Register(f); err != nil {
			panic(err)
		}
	}
	return &Engine{
		registry:          registry,
		contexts:          map[string]*contextState{},
		neverDropPatterns: normalizeNeverDropPatterns(cfg.NeverDropPatterns),
		auditDisabled:     cfg.DisableAudit,
	}
}

// DefaultNeverDropPatterns returns case-insensitive substrings that should never
// be deduplicated away.
func DefaultNeverDropPatterns() []string {
	return []string{"error", "fatal", "panic", "traceback", "exception", "failed"}
}

// SetCommandID sets the command identifier used in emitted events and audit records.
func (e *Engine) SetCommandID(id string) {
	e.mu.Lock()
	e.commandID = id
	e.mu.Unlock()
}

// Process consumes one line/EOF/tick-equivalent event for a specific stream/tool.
func (e *Engine) Process(stream string, tool string, input Input) Output {
	e.mu.Lock()
	ev := e.buildEvent(stream, tool, input)
	f := e.resolveFilter(tool)
	ctxKey := f.ContextKey(ev)
	ctx := e.contextForEvent(ctxKey, ev, f)
	e.mu.Unlock()

	ctx.mu.Lock()
	ctx.applyEventMeta(ev)
	out := Output{
		Stream: ev.Stream,
		Audit: AuditRecord{
			Sequence:  ev.Sequence,
			CommandID: ev.CommandID,
			Tool:      ev.Tool,
			Stream:    ev.Stream,
		},
	}
	if !e.auditDisabled {
		out.Audit.Raw = ev.Line
	}
	if e.handleLineEvent(ctx, ev, f, &out) {
		ctx.mu.Unlock()
		return out
	}
	if ev.Type == EventEOF {
		ctx.eofMask |= streamMask(ev.Stream)
	}

	d := f.Process(ev, ctx.buffer)
	e.applyDecisionAudit(&out, d)
	out, deleteCtx := e.finalizeDecision(ctx, ev, d, out)
	if deleteCtx {
		e.deleteContextIfCurrent(ctxKey, ctx)
	}
	return out
}

func (e *Engine) buildEvent(stream string, tool string, input Input) Event {
	ev := Event{
		Tool:      tool,
		Dispatch:  input.Dispatch,
		Stream:    StreamType(stream),
		CommandID: e.commandID,
		Sequence:  e.nextSequence(),
	}
	switch {
	case input.Tick:
		ev.Type = EventTick
	case input.Exit:
		ev.Type = EventExit
		ev.ExitCode = input.Code
	case input.EOF:
		ev.Type = EventEOF
	default:
		ev.Type = EventLine
		ev.Line = input.Line
	}
	return ev
}

func (e *Engine) handleLineEvent(ctx *contextState, ev Event, f ToolFilter, out *Output) bool {
	if ev.Type != EventLine {
		return false
	}
	ctx.seenMask |= streamMask(ev.Stream)
	if strings.TrimSpace(ev.Line) != "" {
		e.collectLine(ctx, ev, f, out)
	}
	if ctx.buffer.Len() <= maxBufferedLines {
		return false
	}
	elided := ctx.buffer.Len() - maxBufferedLines
	out.Audit.Action = ActionFlush
	out.Ready = true
	out.Output = ctx.buffer.Joined() + fmt.Sprintf("[..., context limit reached; %d lines elided ...]\n", elided)
	ctx.buffer.Clear()
	return true
}

func (e *Engine) collectLine(ctx *contextState, ev Event, f ToolFilter, out *Output) {
	key := e.makeKey(ev.Line, f)
	if !e.auditDisabled {
		e.recordLineAudit(ctx, ev.Line, key, out)
	}
	// Never-drop lines should remain visible even when duplicated.
	if e.matchesNeverDrop(ev.Line) {
		key = key + "#seq:" + strconv.FormatUint(ev.Sequence, 10)
	}
	_ = ctx.buffer.Add(ev.Line, key, ev.Sequence)
}

func (e *Engine) recordLineAudit(ctx *contextState, line string, key string, out *Output) {
	out.Audit.DerivedKey = key
	out.Audit.Mask = "baseline"
	if prev, ok := ctx.rawByKey[key]; ok {
		out.Audit.Collision = prev != line
		return
	}
	if ctx.rawKeyFull {
		return
	}
	ctx.rawByKey[key] = line
	if len(ctx.rawByKey) >= maxRawByKeyEntries {
		ctx.rawKeyFull = true
	}
}

func (e *Engine) applyDecisionAudit(out *Output, d Decision) {
	if e.auditDisabled {
		out.Audit.Action = d.Action
		return
	}
	if d.Key != "" {
		out.Audit.DerivedKey = d.Key
	}
	if d.Mask != "" {
		out.Audit.Mask = d.Mask
	}
	out.Audit.Action = d.Action
	out.Audit.Collision = out.Audit.Collision || d.Collision
}

func (e *Engine) finalizeDecision(ctx *contextState, ev Event, d Decision, out Output) (Output, bool) {
	deleteCtx := false
	switch d.Action {
	case ActionImmediate:
		out.Ready = true
		out.Output = d.Output
	case ActionCollect:
		out.Collect = true
		deleteCtx = ev.Type == EventExit
	case ActionFlush:
		if ev.Type == EventEOF && ctx.shared && !e.contextComplete(ctx) {
			ctx.mu.Unlock()
			return out, false
		}
		out.Output = d.Output
		if out.Output == "" {
			out.Output = ctx.buffer.Joined()
		}
		out.Stream = ctx.stream
		if e.contextComplete(ctx) {
			deleteCtx = true
		} else {
			ctx.buffer.Clear()
		}
		out.Ready = out.Output != ""
	case ActionIgnore:
		deleteCtx = ev.Type == EventExit
	}
	ctx.mu.Unlock()
	return out, deleteCtx
}

// ProcessTick triggers periodic tick processing for currently active contexts.
func (e *Engine) ProcessTick(_ string) []Output {
	e.mu.Lock()

	type tickWork struct {
		ctx *contextState
		f   ToolFilter
		ev  Event
	}
	work := make([]tickWork, 0, len(e.contexts))
	for _, ctx := range e.contexts {
		ctx.mu.Lock()
		tool := ctx.tool
		dispatch := ctx.dispatch
		stream := ctx.stream
		ctx.mu.Unlock()
		work = append(work, tickWork{
			ctx: ctx,
			f:   e.resolveFilter(tool),
			ev: Event{
				Type:      EventTick,
				Tool:      tool,
				Dispatch:  dispatch,
				Stream:    stream,
				CommandID: e.commandID,
				Sequence:  e.nextSequence(),
			},
		})
	}
	e.mu.Unlock()

	outputs := make([]Output, 0, len(work))
	for _, item := range work {
		item.ctx.mu.Lock()
		d := item.f.Process(item.ev, item.ctx.buffer)
		if d.Action != ActionFlush {
			item.ctx.mu.Unlock()
			continue
		}
		out := d.Output
		if out == "" {
			out = item.ctx.buffer.Joined()
		}
		if out == "" {
			item.ctx.mu.Unlock()
			continue
		}
		item.ctx.buffer.Clear()
		item.ctx.mu.Unlock()
		outputs = append(outputs, Output{
			Ready:  true,
			Output: out,
			Stream: item.ev.Stream,
			Audit: AuditRecord{
				Sequence:  item.ev.Sequence,
				CommandID: item.ev.CommandID,
				Tool:      item.ev.Tool,
				Stream:    item.ev.Stream,
				Action:    ActionFlush,
			},
		})
		// Contexts stay alive on tick flush and are cleaned at complete EOF.
	}
	return outputs
}

func (e *Engine) deleteContextIfCurrent(ctxKey string, want *contextState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cur, ok := e.contexts[ctxKey]; ok && cur == want {
		delete(e.contexts, ctxKey)
	}
}

// RegisterFilter adds a new tool filter to the engine registry.
func (e *Engine) RegisterFilter(f ToolFilter) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.registry.Register(f)
}

func (e *Engine) resolveFilter(tool string) ToolFilter {
	if f := e.registry.Resolve(tool); f != nil {
		return f
	}
	return NewNoopFilter(tool)
}

func (e *Engine) contextForEvent(ctxKey string, ev Event, f ToolFilter) *contextState {
	if ctx, ok := e.contexts[ctxKey]; ok {
		return ctx
	}
	if len(e.contexts) >= maxActiveContexts {
		e.evictOldestContext()
	}
	shared := false
	if ev.Stream == StdoutStream || ev.Stream == StderrStream {
		other := ev
		if ev.Stream == StdoutStream {
			other.Stream = StderrStream
		} else {
			other.Stream = StdoutStream
		}
		shared = f.ContextKey(other) == ctxKey
	}
	ctx := newContextState(ev, shared)
	e.contexts[ctxKey] = ctx
	return ctx
}

func (e *Engine) makeKey(line string, f ToolFilter) string {
	line = strings.TrimRight(line, " \t\r\n")
	h := f.MaskingHorizon()
	if h == 0 {
		h = 1024
	}
	if h > 0 && len(line) > h {
		line = line[:h]
	}
	return maskLine(line)
}

func (e *Engine) matchesNeverDrop(line string) bool {
	if len(e.neverDropPatterns) == 0 {
		return false
	}
	lower := strings.ToLower(line)
	for _, p := range e.neverDropPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (e *Engine) contextComplete(ctx *contextState) bool {
	return ctx.seenMask == 0 || ctx.eofMask&ctx.seenMask == ctx.seenMask
}

func (e *Engine) nextSequence() uint64 {
	e.sequence++
	return e.sequence
}

func (e *Engine) evictOldestContext() {
	if len(e.contexts) == 0 {
		return
	}
	var oldestKey string
	var oldest *contextState
	for k, ctx := range e.contexts {
		if oldest == nil || ctx.lastSeq < oldest.lastSeq {
			oldestKey = k
			oldest = ctx
		}
	}
	if oldest != nil {
		delete(e.contexts, oldestKey)
	}
}

func normalizeNeverDropPatterns(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func streamMask(stream StreamType) uint8 {
	switch stream {
	case StdoutStream:
		return 1
	case StderrStream:
		return 2
	default:
		return 0
	}
}
