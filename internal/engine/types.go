package engine

import "runtime"

type EventType string

const (
	// EventLine carries one textual line from a stream.
	EventLine EventType = "line"
	// EventEOF marks stream completion for a context.
	EventEOF EventType = "eof"
	// EventExit marks command completion with process exit metadata.
	EventExit EventType = "exit"
	// EventTick is a synthetic timer event for stale-context handling.
	EventTick EventType = "tick"
)

// StreamType identifies stdout or stderr origin.
type StreamType string

const (
	// StdoutStream denotes standard output.
	StdoutStream StreamType = "stdout"
	// StderrStream denotes standard error.
	StderrStream StreamType = "stderr"
)

// Event is the normalized engine event sent to a ToolFilter.
type Event struct {
	Type      EventType
	Line      string
	Tool      string
	Dispatch  string
	Stream    StreamType
	CommandID string
	Sequence  uint64
	ExitCode  int
}

// ExecPlan is the prepared execution plan produced by runner planning.
type ExecPlan struct {
	Tool            string
	DispatchKey     string
	Name            string
	Args            []string
	FallbackName    string
	FallbackArgs    []string
	RawInput        string
	IsAmbiguous     bool
	AmbiguityReason string
	AmbiguityOps    []string
}

// ShellCommandForOS returns the shell executable used for ambiguous commands.
func ShellCommandForOS(goos string) string {
	if goos == "windows" {
		return "cmd.exe"
	}
	return "sh"
}

// ShellArgsForOS returns shell arguments for executing one command line.
func ShellArgsForOS(goos, cmdline string) []string {
	if goos == "windows" {
		return []string{"/C", cmdline}
	}
	return []string{"-lc", cmdline}
}

// ShellCommand returns the shell executable for the current runtime OS.
func ShellCommand() string {
	return ShellCommandForOS(runtime.GOOS)
}

// ShellArgs returns shell arguments for the current runtime OS.
func ShellArgs(cmdline string) []string { return ShellArgsForOS(runtime.GOOS, cmdline) }

// Action defines how the engine should handle a filter decision.
type Action string

const (
	// ActionCollect stores line data and emits nothing yet.
	ActionCollect Action = "collect"
	// ActionImmediate emits output right away.
	ActionImmediate Action = "immediate"
	// ActionFlush emits buffered output.
	ActionFlush Action = "flush"
	// ActionIgnore drops the event output path.
	ActionIgnore Action = "ignore"
)

// Decision is returned by a ToolFilter for each event.
type Decision struct {
	Action    Action
	Output    string
	Key       string
	Mask      string
	Collision bool
}

// AuditRecord captures trace data for diagnostics.
type AuditRecord struct {
	Sequence   uint64     `json:"sequence"`
	CommandID  string     `json:"commandId"`
	Tool       string     `json:"tool"`
	Stream     StreamType `json:"stream"`
	Raw        string     `json:"raw"`
	DerivedKey string     `json:"derivedKey"`
	Mask       string     `json:"mask"`
	Action     Action     `json:"action"`
	Collision  bool       `json:"collisionBit"`
}

// PrepareResult returns filter-driven command normalization metadata.
type PrepareResult struct {
	NormalizedArgs        []string
	DispatchKey           string
	Ambiguous             bool
	Reason                string
	ForcePassthrough      bool
	PreferredSubstitution string
	PreferredArgs         []string
	FallbackArgs          []string
}

// ToolFilter defines command preparation and output-compaction behavior per tool.
type ToolFilter interface {
	Tool() string
	Aliases() []string
	Prepare(args []string) PrepareResult
	ContextKey(ev Event) string
	Process(ev Event, mem *OrderedSetBuffer) Decision
	MaskingHorizon() int
}

// StreamContextKey returns a stable key for stream-scoped filter contexts.
func StreamContextKey(commandID, tool string, stream StreamType) string {
	return commandID + ":" + tool + ":" + string(stream)
}

// SharedContextKey returns a stable key for filters that must share stdout/stderr memory.
func SharedContextKey(ev Event) string {
	return SharedContextKeyForTool(ev.CommandID, ev.Tool)
}

// SharedContextKeyForTool returns a stable key for shared context when a filter
// needs to pin to a delegated tool name.
func SharedContextKeyForTool(commandID, tool string) string {
	return StreamContextKey(commandID, tool, StreamType("shared"))
}
