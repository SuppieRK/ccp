package contracts

type Stream string

const (
	StreamCombined Stream = "combined"
	StreamStdout   Stream = "stdout"
	StreamStderr   Stream = "stderr"
)

type Command struct {
	CommandID    string
	RawInput     string
	Args         []string
	MatchingArgs []string
	Tool         string
	Dispatch     string
}

func (c Command) ArgsForMatching() []string {
	if c.MatchingArgs != nil {
		return c.MatchingArgs
	}
	return c.Args
}

type FilterProvenance struct {
	SourceKind string
	Path       string
	Hash       string
}

type FilterSourceBuildTiming struct {
	SourceKind  string
	SourceDir   string
	Definitions int64
	Compiled    int64
	DurationMS  int64
	Error       string
}

type FilterRegistryBuildTiming struct {
	DurationMS int64
	Sources    []FilterSourceBuildTiming
}

type ActionKind string

const (
	ActionKeep    ActionKind = "keep"
	ActionEmit    ActionKind = "emit"
	ActionIgnore  ActionKind = "ignore"
	ActionReplace ActionKind = "replace"
)

type Action struct {
	Kind         ActionKind
	Stream       Stream
	Output       string
	ReplaceCount int
}

type Context interface {
	Args() []string
	BufferedCount(stream Stream) int
	BufferedLines(stream Stream) []string
	ExitCode() int
}

type Filter interface {
	PrepareCommand(command Command) (Command, error)
	Dispatch(command Command) string
	OnStdout(line string, context Context) Action
	OnStderr(line string, context Context) Action
	OnStdoutExit(context Context) Action
}

// ExitActionsFilter optionally finalizes multiple output streams atomically.
type ExitActionsFilter interface {
	OnStdoutExitActions(context Context) []Action
}

type CloneableFilter interface {
	CloneFilter() Filter
}

type ProvenanceFilter interface {
	FilterProvenance() FilterProvenance
}
