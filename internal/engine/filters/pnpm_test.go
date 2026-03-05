package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const pnpmInstallDispatchKey = "pnpm|mode=install"

func TestPNPMMetadataAndAliases(t *testing.T) {
	f := NewPNPMFilter()
	if f.Tool() != "pnpm" {
		t.Fatalf("expected pnpm tool, got %q", f.Tool())
	}
	want := []string{"pnpm.cmd", "./pnpm.cmd", "pnpm.exe", "./pnpm.exe"}
	if !slices.Equal(f.Aliases(), want) {
		t.Fatalf("unexpected aliases: want=%#v got=%#v", want, f.Aliases())
	}
}

func TestPNPMPrepareRoutesSupportedSubcommands(t *testing.T) {
	f := NewPNPMFilter()

	list := f.Prepare([]string{"list"})
	if list.ForcePassthrough {
		t.Fatal("expected list route")
	}
	if list.DispatchKey != "pnpm|mode=list" {
		t.Fatalf("unexpected list dispatch key: %q", list.DispatchKey)
	}
	if got := strings.Join(list.NormalizedArgs, " "); !strings.Contains(got, "--json") || !strings.Contains(got, "--depth=0") {
		t.Fatalf("expected list normalization, got %#v", list.NormalizedArgs)
	}

	listKeep := f.Prepare([]string{"list", "--json", "--depth", "2"})
	if !listKeep.ForcePassthrough || !listKeep.Ambiguous {
		t.Fatalf("expected explicit list json structured passthrough, got %#v", listKeep)
	}

	outdated := f.Prepare([]string{"outdated"})
	if outdated.ForcePassthrough {
		t.Fatal("expected outdated route")
	}
	if outdated.DispatchKey != "pnpm|mode=outdated" {
		t.Fatalf("unexpected outdated dispatch key: %q", outdated.DispatchKey)
	}
	if got := strings.Join(outdated.NormalizedArgs, " "); !strings.Contains(got, "--format json") {
		t.Fatalf("expected outdated format normalization, got %#v", outdated.NormalizedArgs)
	}

	outdatedKeep := f.Prepare([]string{"outdated", "--format", "json"})
	if !outdatedKeep.ForcePassthrough || !outdatedKeep.Ambiguous {
		t.Fatalf("expected explicit outdated json structured passthrough, got %#v", outdatedKeep)
	}

	install := f.Prepare([]string{"install", "@scope/pkg@1.2.3"})
	if install.ForcePassthrough {
		t.Fatal("expected install route")
	}
	if install.DispatchKey != pnpmInstallDispatchKey {
		t.Fatalf("unexpected install dispatch key: %q", install.DispatchKey)
	}
}

func TestPNPMPrepareUnsupportedPassthrough(t *testing.T) {
	f := NewPNPMFilter()
	for _, args := range [][]string{{}, {"test"}, {"exec", "node"}} {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough {
			t.Fatalf("expected passthrough for args=%#v", args)
		}
	}
}

func TestPNPMInstallSafeNameValidation(t *testing.T) {
	f := NewPNPMFilter()
	good := [][]string{{"install", "lodash"}, {"install", "@clerk/express"}, {"install", "typescript@5.6.2"}}
	for _, args := range good {
		prep := f.Prepare(args)
		if prep.ForcePassthrough || prep.Ambiguous {
			t.Fatalf("expected safe package accepted for args=%#v got=%#v", args, prep)
		}
	}

	bad := [][]string{{"install", "../etc/passwd"}, {"install", "lodash;rm"}, {"install", "..\\evil"}}
	for _, args := range bad {
		prep := f.Prepare(args)
		if !prep.ForcePassthrough || !prep.Ambiguous {
			t.Fatalf("expected unsafe package rejected for args=%#v got=%#v", args, prep)
		}
	}
}

func TestCompactPNPMListTiered(t *testing.T) {
	jsonIn := `[{"name":"app","version":"1.0.0","dependencies":{"lodash":{"version":"4.17.21"}}}]`
	out, ok := compactPNPMOutput(jsonIn, pnpmDispatch{mode: "list"}, 0)
	if !ok || !strings.Contains(out, "dependencies: 2") {
		t.Fatalf("expected structured list compaction, got ok=%v out=%q", ok, out)
	}

	textIn := "app@1.0.0\nlodash@4.17.21\n"
	out, ok = compactPNPMOutput(textIn, pnpmDispatch{mode: "list"}, 0)
	if !ok || !strings.Contains(out, "dependencies: 2") {
		t.Fatalf("expected degraded list compaction, got ok=%v out=%q", ok, out)
	}

	fallback := "<<<<not parseable>>>>"
	_, ok = compactPNPMOutput(fallback, pnpmDispatch{mode: "list"}, 0)
	if ok {
		t.Fatalf("expected list passthrough fallback for %q", fallback)
	}
}

func TestCompactPNPMOutdatedTieredAndSuccessMarkerGate(t *testing.T) {
	jsonIn := `[{"name":"express","current":"4.18.2","wanted":"4.18.2","latest":"4.19.0"}]`
	out, ok := compactPNPMOutput(jsonIn, pnpmDispatch{mode: "outdated"}, 0)
	if !ok || !strings.Contains(out, "outdated: 1/1") || !strings.Contains(out, "express") {
		t.Fatalf("expected structured outdated compaction, got ok=%v out=%q", ok, out)
	}

	textIn := "Package Current Wanted Latest\nexpress 4.18.2 4.18.2 4.19.0\n"
	out, ok = compactPNPMOutput(textIn, pnpmDispatch{mode: "outdated"}, 0)
	if !ok || !strings.Contains(out, "outdated: 1/1") {
		t.Fatalf("expected degraded outdated compaction, got ok=%v out=%q", ok, out)
	}

	emptyJSON := "[]"
	out, ok = compactPNPMOutput(emptyJSON, pnpmDispatch{mode: "outdated"}, 0)
	if !ok || strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty outdated output for success marker path, got ok=%v out=%q", ok, out)
	}

	out, ok = compactPNPMOutput("[]", pnpmDispatch{mode: "outdated"}, 1)
	if ok && strings.Contains(out, "All packages up-to-date") {
		t.Fatalf("expected no success marker on non-zero exit, got %q", out)
	}
}

func TestCompactPNPMInstallNoiseAndFailureRetention(t *testing.T) {
	raw := strings.Join([]string{
		"Progress: resolved 100, reused 1, downloaded 2, added 3",
		"Packages: +1",
		"dependencies:",
		"+ lodash 4.17.21",
		"Done in 2.0s",
	}, "\n") + "\n"
	out, ok := compactPNPMOutput(raw, pnpmDispatch{mode: "install"}, 0)
	if !ok {
		t.Fatal("expected install compaction")
	}
	if strings.Contains(out, "Progress") || strings.Contains(out, "Packages:") {
		t.Fatalf("expected progress suppression, got %q", out)
	}
	if !strings.Contains(out, "+ lodash") {
		t.Fatalf("expected summary retained, got %q", out)
	}

	raw = "ERR_PNPM_FETCH_404 GET https://registry.npmjs.org/foo\nProgress: resolved 1\n"
	out, ok = compactPNPMOutput(raw, pnpmDispatch{mode: "install"}, 1)
	if !ok || !strings.Contains(out, "ERR_PNPM_FETCH_404") {
		t.Fatalf("expected failure retention, got ok=%v out=%q", ok, out)
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("unexpected success marker on failure: %q", out)
	}
}

func TestPNPMExitSuccessMarkers(t *testing.T) {
	f := NewPNPMFilter()
	tests := []struct {
		name     string
		dispatch string
		exitCode int
		want     string
	}{
		{name: "install success marker", dispatch: pnpmInstallDispatchKey, exitCode: 0, want: "ok"},
		{name: "outdated success marker", dispatch: "pnpm|mode=outdated", exitCode: 0, want: "All packages up-to-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := f.Process(engine.Event{Type: engine.EventExit, Tool: "pnpm", Dispatch: tt.dispatch, ExitCode: tt.exitCode}, engine.NewOrderedSetBuffer())
			if d.Action != engine.ActionFlush || strings.TrimSpace(d.Output) != tt.want {
				t.Fatalf("expected %q marker, got %#v", tt.want, d)
			}
		})
	}

	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "pnpm", Dispatch: pnpmInstallDispatchKey, ExitCode: 1}, engine.NewOrderedSetBuffer())
	if d.Action == engine.ActionFlush && strings.Contains(d.Output, "ok") {
		t.Fatalf("unexpected ok marker on non-zero exit: %#v", d)
	}
}

func TestPNPMLowConfidenceFallback(t *testing.T) {
	f := NewPNPMFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("\x00\x01\x02\x03\x04\n", "", 1)
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "pnpm", Dispatch: "pnpm|mode=list", ExitCode: 0}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush passthrough, got %q", d.Action)
	}
	if !strings.Contains(d.Output, "\x00") {
		t.Fatalf("expected raw passthrough output, got %q", d.Output)
	}
}

func TestPNPMProcessCollectsPreExitEvents(t *testing.T) {
	f := NewPNPMFilter()
	mem := engine.NewOrderedSetBuffer()
	for _, ev := range []engine.Event{
		{Type: engine.EventLine, Tool: "pnpm", Dispatch: "pnpm|mode=list", Stream: engine.StdoutStream, Line: "payload\n"},
		{Type: engine.EventTick, Tool: "pnpm", Dispatch: "pnpm|mode=list", Stream: engine.StdoutStream},
		{Type: engine.EventEOF, Tool: "pnpm", Dispatch: "pnpm|mode=list", Stream: engine.StdoutStream},
	} {
		d := f.Process(ev, mem)
		if d.Action != engine.ActionCollect {
			t.Fatalf("expected collect for event=%v, got %#v", ev.Type, d)
		}
	}
}

func TestPNPMExitEmptyNonZeroIgnores(t *testing.T) {
	f := NewPNPMFilter()
	d := f.Process(engine.Event{Type: engine.EventExit, Tool: "pnpm", Dispatch: "pnpm|mode=outdated", ExitCode: 1}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionIgnore || d.Output != "" {
		t.Fatalf("expected ignore/no output for empty non-zero exit, got %#v", d)
	}
}
