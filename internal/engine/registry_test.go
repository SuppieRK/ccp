package engine

import "testing"

func TestRegistryResolveByToolAndAlias(t *testing.T) {
	r := NewToolFilterRegistry()
	f := registryTestFilter{engineTestFilterBase: engineTestFilterBase{tool: "git", aliases: []string{"g"}}}
	if err := r.Register(f); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := r.Resolve("git"); got == nil || got.Tool() != "git" {
		t.Fatalf("resolve tool failed: %#v", got)
	}
	if got := r.Resolve("G"); got == nil || got.Tool() != "git" {
		t.Fatalf("resolve alias failed: %#v", got)
	}
}

func TestRegistryResolveUnknown(t *testing.T) {
	r := NewToolFilterRegistry()
	if got := r.Resolve("missing"); got != nil {
		t.Fatalf("expected nil for unknown, got %#v", got)
	}
}

func TestRegistryDuplicateToolFailsFast(t *testing.T) {
	r := NewToolFilterRegistry()
	if err := r.Register(registryTestFilter{engineTestFilterBase: engineTestFilterBase{tool: "ls"}}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if r.Register(registryTestFilter{engineTestFilterBase: engineTestFilterBase{tool: "ls"}}) == nil {
		t.Fatal("expected duplicate tool error")
	}
}

func TestRegistryDuplicateAliasFailsFast(t *testing.T) {
	r := NewToolFilterRegistry()
	if err := r.Register(registryTestFilter{engineTestFilterBase: engineTestFilterBase{tool: "a", aliases: []string{"x"}}}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if r.Register(registryTestFilter{engineTestFilterBase: engineTestFilterBase{tool: "b", aliases: []string{"x"}}}) == nil {
		t.Fatal("expected duplicate alias error")
	}
}

func TestIsNormalizedLookupKey(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"git", true},
		{"git log", true},
		{"", false},
		{" Git", false},
		{"git ", false},
		{"Git", false},
	}
	for _, tc := range tests {
		if got := isNormalizedLookupKey(tc.in); got != tc.want {
			t.Fatalf("isNormalizedLookupKey(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

type registryTestFilter struct {
	engineTestFilterBase
}

func (f registryTestFilter) Process(ev Event, _ *OrderedSetBuffer) Decision {
	return Decision{Action: ActionCollect}
}
