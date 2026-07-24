package filters

import "github.com/SuppieRK/cmdshape/internal/contracts"

type Passthrough struct{}

func (Passthrough) PrepareCommand(command contracts.Command) (contracts.Command, error) {
	return command, nil
}

func (Passthrough) Dispatch(command contracts.Command) string {
	return command.Tool
}

func (Passthrough) OnStdout(_ string, _ contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionEmit}
}

func (Passthrough) OnStderr(_ string, _ contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionEmit}
}

func (Passthrough) OnStdoutExit(contracts.Context) contracts.Action {
	return contracts.Action{Kind: contracts.ActionEmit}
}
