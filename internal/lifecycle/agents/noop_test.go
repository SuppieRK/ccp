package agents

import (
	"strings"
	"testing"
)

func TestNoopAdapter(t *testing.T) {
	a := NewNoopAdapter("noop", ".noop")
	ctx := Context{ScopeRoot: t.TempDir(), HomeDir: t.TempDir()}
	if a.ID() != "noop" {
		t.Fatalf(errUnexpectedIDFmt, a.ID())
	}
	if !strings.Contains(a.DetectRoot(ctx.ScopeRoot), ".noop") {
		t.Fatalf(errUnexpectedRootFmt, a.DetectRoot(ctx.ScopeRoot))
	}
	if plan := a.Plan(ctx); len(plan) != 0 {
		t.Fatalf("expected empty noop plan, got %+v", plan)
	}
	res, err := a.Install(ctx, writeFileWriter)
	if err != nil || res.Noop != 1 {
		t.Fatalf("unexpected noop install result %+v err=%v", res, err)
	}
	if err := a.Verify(ctx); err != nil {
		t.Fatalf("unexpected noop verify error: %v", err)
	}
	res, err = a.Uninstall(ctx)
	if err != nil || res.Noop != 1 {
		t.Fatalf("unexpected noop uninstall result %+v err=%v", res, err)
	}
}
