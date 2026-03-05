package engine

type engineTestFilterBase struct {
	tool          string
	aliases       []string
	sharedContext bool
}

func (b engineTestFilterBase) Tool() string      { return b.tool }
func (b engineTestFilterBase) Aliases() []string { return b.aliases }
func (b engineTestFilterBase) Prepare(args []string) PrepareResult {
	return PrepareResult{NormalizedArgs: args}
}
func (b engineTestFilterBase) ContextKey(ev Event) string {
	if b.sharedContext {
		return SharedContextKey(ev)
	}
	return StreamContextKey(ev.CommandID, ev.Tool, ev.Stream)
}
func (b engineTestFilterBase) MaskingHorizon() int { return 0 }
