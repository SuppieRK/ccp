package contracts

type Stream string

const (
	StreamCombined Stream = "combined"
	StreamStdout   Stream = "stdout"
	StreamStderr   Stream = "stderr"
)

type Command struct {
	CommandID string
	RawInput  string
	Args      []string
	Tool      string
	Dispatch  string
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
	BufferedLines(stream Stream) []string
}

type Filter interface {
	PrepareCommand(command Command) (Command, error)
	Dispatch(command Command) string
	OnStdout(line string, context Context) Action
	OnStderr(line string, context Context) Action
	OnStdoutExit(context Context) Action
}
