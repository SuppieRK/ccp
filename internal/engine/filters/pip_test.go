package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	pipListDispatchKey = "pip|mode=list"
	pipFormatJSONArg   = "--format=json"
	pipFormatFlag      = "--format"
)

func TestPIPFilterMetadataAndAliases(t *testing.T) {
	f := NewPIPFilter()
	if f.Tool() != "pip" {
		t.Fatalf("expected pip tool, got %q", f.Tool())
	}
	wantContains := []string{"pip3", "pip.exe", "pip3.exe", "pip.cmd", "pip3.cmd"}
	aliases := f.Aliases()
	for _, want := range wantContains {
		found := false
		for _, alias := range aliases {
			if strings.EqualFold(alias, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing alias %q in %#v", want, aliases)
		}
	}
}

func TestPIPPrepareListAndOutdatedStructured(t *testing.T) {
	f := NewPIPFilter()
	list := f.Prepare([]string{"list"})
	if list.ForcePassthrough {
		t.Fatalf("expected list to stay filter-bound, got %#v", list)
	}
	if list.DispatchKey != pipListDispatchKey {
		t.Fatalf("unexpected list dispatch key: %q", list.DispatchKey)
	}
	if !slices.Equal(list.NormalizedArgs, []string{"list", pipFormatJSONArg}) {
		t.Fatalf("unexpected list args: %#v", list.NormalizedArgs)
	}
	if list.PreferredSubstitution != "uv" {
		t.Fatalf("expected uv preferred substitution, got %q", list.PreferredSubstitution)
	}
	if !slices.Equal(list.PreferredArgs, []string{"pip", "list", pipFormatJSONArg}) {
		t.Fatalf("unexpected list preferred args: %#v", list.PreferredArgs)
	}

	outdated := f.Prepare([]string{"outdated"})
	if outdated.DispatchKey != "pip|mode=outdated" {
		t.Fatalf("unexpected outdated dispatch key: %q", outdated.DispatchKey)
	}
	if !slices.Equal(outdated.NormalizedArgs, []string{"outdated", "--outdated", pipFormatJSONArg}) {
		t.Fatalf("unexpected outdated args: %#v", outdated.NormalizedArgs)
	}
	if !slices.Equal(outdated.PreferredArgs, []string{"pip", "list", "--outdated", pipFormatJSONArg}) {
		t.Fatalf("unexpected outdated preferred args: %#v", outdated.PreferredArgs)
	}
}

func TestPIPPrepareFormatConflictFallsBackToPassthrough(t *testing.T) {
	f := NewPIPFilter()
	prep := f.Prepare([]string{"list", "--format=freeze"})
	if !prep.ForcePassthrough || !prep.Ambiguous {
		t.Fatalf("expected ambiguous passthrough for format conflict, got %#v", prep)
	}
	prep = f.Prepare([]string{"outdated", pipFormatFlag, "columns"})
	if !prep.ForcePassthrough || !prep.Ambiguous {
		t.Fatalf("expected ambiguous passthrough for format conflict, got %#v", prep)
	}
}

func TestPIPPrepareExplicitJSONFormatPassthrough(t *testing.T) {
	f := NewPIPFilter()
	for _, tc := range [][]string{
		{"list", pipFormatJSONArg},
		{"list", pipFormatFlag, "json"},
		{"outdated", pipFormatJSONArg},
		{"outdated", pipFormatFlag, "json"},
	} {
		prep := f.Prepare(tc)
		if !prep.ForcePassthrough || !prep.Ambiguous {
			t.Fatalf("expected structured passthrough for %#v, got %#v", tc, prep)
		}
	}
}

func TestPIPPreparePassthroughSubcommands(t *testing.T) {
	f := NewPIPFilter()
	for _, tc := range [][]string{
		nil,
		{"install", "requests"},
		{"uninstall", "requests"},
		{"show", "requests"},
		{"wheel", "requests"},
	} {
		prep := f.Prepare(tc)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for %#v, got %#v", tc, prep)
		}
	}
}

func TestPIPPrepareCompatibilitySensitiveFlagsAreAmbiguousPassthrough(t *testing.T) {
	f := NewPIPFilter()
	for _, tc := range [][]string{{"install", "--editable", "."}, {"list", "--user"}, {"install", "-e", "."}} {
		prep := f.Prepare(tc)
		if !prep.ForcePassthrough || !prep.Ambiguous {
			t.Fatalf("expected ambiguous passthrough for %#v, got %#v", tc, prep)
		}
		if prep.PreferredSubstitution != "" {
			t.Fatalf("expected no substitution for %#v, got %#v", tc, prep)
		}
	}
}

func TestPIPContextKeySharedAcrossStreams(t *testing.T) {
	f := NewPIPFilter()
	stdout := f.ContextKey(engine.Event{CommandID: "pip list", Tool: "pip", Stream: engine.StdoutStream})
	stderr := f.ContextKey(engine.Event{CommandID: "pip list", Tool: "pip", Stream: engine.StderrStream})
	if stdout != stderr {
		t.Fatalf("expected shared context key, got %q != %q", stdout, stderr)
	}
}

func TestPIPCompactionListAndOutdatedAndFallbacks(t *testing.T) {
	listRaw := `[ {"name":"requests","version":"2.31.0"}, {"name":"pytest","version":"8.3.0"} ]\n`
	listOut, ok := compactPIPOutput(listRaw, "list")
	if !ok || !strings.Contains(listOut, "pip list: 2 packages") || !strings.Contains(listOut, "pytest (8.3.0)") {
		t.Fatalf("unexpected list compact output: ok=%v out=%q", ok, listOut)
	}

	outdatedRaw := `[ {"name":"django","version":"4.2.0","latest_version":"5.0.0"} ]\n`
	outdatedOut, ok := compactPIPOutput(outdatedRaw, "outdated")
	if !ok || !strings.Contains(outdatedOut, "pip outdated: 1 packages") || !strings.Contains(outdatedOut, "django 4.2.0 -> 5.0.0") {
		t.Fatalf("unexpected outdated compact output: ok=%v out=%q", ok, outdatedOut)
	}

	variantRaw := `[ {"name":"flask","current_version":"2.3.0","latest":"3.0.0"} ]\n`
	variantOut, ok := compactPIPOutput(variantRaw, "outdated")
	if !ok || !strings.Contains(variantOut, "flask 2.3.0 -> 3.0.0") {
		t.Fatalf("expected variant field normalization, got ok=%v out=%q", ok, variantOut)
	}

	malformed := `warning: fallback\nnot json\n`
	fallback, ok := compactPIPOutput(malformed, "list")
	if ok || fallback != malformed {
		t.Fatalf("expected malformed fallback raw output, got ok=%v out=%q", ok, fallback)
	}
}

func TestPIPProcessExitCompactsOrFallsBack(t *testing.T) {
	f := NewPIPFilter()
	mem := engine.NewOrderedSetBuffer()
	raw := `[ {"name":"requests","version":"2.31.0"} ]\n`
	_ = mem.Add(raw, raw, 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: pipListDispatchKey, ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush || !strings.Contains(d.Output, "pip list: 1 packages") {
		t.Fatalf("expected compact flush on exit, got %#v", d)
	}

	mem2 := engine.NewOrderedSetBuffer()
	bad := "non-json\n"
	_ = mem2.Add(bad, bad, 1)
	d = f.Process(engine.Event{Type: engine.EventExit, Dispatch: pipListDispatchKey, ExitCode: 1}, mem2)
	if d.Action != engine.ActionFlush || d.Output != bad {
		t.Fatalf("expected raw fallback flush on malformed payload, got %#v", d)
	}
}

func TestPIPProcessCollectsPreExitEvents(t *testing.T) {
	f := NewPIPFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Dispatch: pipListDispatchKey, Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Dispatch: pipListDispatchKey, Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Dispatch: pipListDispatchKey, Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestPIPProcessExitEmptyIgnores(t *testing.T) {
	f := NewPIPFilter()
	d := f.Process(engine.Event{Type: engine.EventExit, Dispatch: pipListDispatchKey, ExitCode: 0}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore for empty exit output, got %#v", d)
	}
}

func TestPIPCompactionPreservesEnvelope(t *testing.T) {
	raw := "warning: prefix\n[ {\"name\":\"requests\",\"version\":\"2.31.0\"} ]\nnotice: suffix\n"
	out, ok := compactPIPOutput(raw, "list")
	if !ok {
		t.Fatalf("expected compact output with preserved envelope")
	}
	if !strings.Contains(out, "warning: prefix") || !strings.Contains(out, "notice: suffix") {
		t.Fatalf("expected prefix/suffix preserved, got %q", out)
	}
	if !strings.Contains(out, "pip list: 1 packages") {
		t.Fatalf("expected compact list summary retained, got %q", out)
	}
}

func TestPIPOutdatedRequiredFieldFallbackRaw(t *testing.T) {
	raw := `[ {"name":"django","version":"4.2.0"} ]` + "\n"
	out, ok := compactPIPOutput(raw, "outdated")
	if ok {
		t.Fatalf("expected fallback for missing latest field, got ok=true out=%q", out)
	}
	if !strings.Contains(out, `"name":"django"`) {
		t.Fatalf("expected payload passthrough on fallback, got out=%q", out)
	}
}

func assertAliasesContainFold(t *testing.T, aliases, expected []string) {
	t.Helper()
	for _, want := range expected {
		found := false
		for _, alias := range aliases {
			if strings.EqualFold(alias, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing alias %q in %#v", want, aliases)
		}
	}
}
