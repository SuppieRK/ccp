package filters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const dockerPSDispatch = "docker ps"

func TestDockerParentPrepareRouting(t *testing.T) {
	f := NewDockerToolFilter()
	cases := []struct {
		name     string
		args     []string
		dispatch string
	}{
		{name: "ps", args: []string{"ps"}, dispatch: dockerPSDispatch},
		{name: "compose ps", args: []string{"compose", "ps"}, dispatch: "docker compose ps"},
		{name: "compose logs", args: []string{"compose", "logs", "--tail", "50", "api"}, dispatch: "docker compose logs|scope=api"},
		{name: "compose logs with file", args: []string{"compose", "-f", "compose.yml", "logs", "api"}, dispatch: "docker compose logs|scope=api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough || prep.DispatchKey != tc.dispatch {
				t.Fatalf("expected dispatch %q, got %#v", tc.dispatch, prep)
			}
		})
	}
}

func TestDockerParentPrepareImagesNormalization(t *testing.T) {
	f := NewDockerToolFilter()

	images := f.Prepare([]string{"images"})
	if images.ForcePassthrough || images.DispatchKey != "docker images" {
		t.Fatalf("expected docker images dispatch, got %#v", images)
	}
	if len(images.NormalizedArgs) != 3 ||
		images.NormalizedArgs[0] != "images" ||
		images.NormalizedArgs[1] != "--format" ||
		images.NormalizedArgs[2] != dockerImagesStructuredFormat {
		t.Fatalf("expected docker images structured normalization, got %#v", images.NormalizedArgs)
	}

}

func TestDockerParentPrepareLogsDispatch(t *testing.T) {
	f := NewDockerToolFilter()

	logs := f.Prepare([]string{"logs", "--tail", "50", "web-1"})
	if logs.ForcePassthrough || !strings.HasPrefix(logs.DispatchKey, "docker logs|container=web-1") {
		t.Fatalf("expected docker logs dispatch with container identity, got %#v", logs)
	}
}

func TestDockerParentPreparePassthroughCases(t *testing.T) {
	f := NewDockerToolFilter()
	cases := []struct {
		name          string
		args          []string
		wantAmbiguous bool
	}{
		{name: "compose-format", args: []string{"compose", "ps", "--format", "json"}, wantAmbiguous: true},
		{name: "exec", args: []string{"exec", "-it", "c1", "sh"}, wantAmbiguous: true},
		{name: "pull", args: []string{"pull", "alpine:latest"}, wantAmbiguous: true},
		{name: "build", args: []string{"build", "."}, wantAmbiguous: true},
		{name: "logs-follow", args: []string{"logs", "web-1", "-f"}, wantAmbiguous: true},
		{name: "compose-logs-follow", args: []string{"compose", "logs", "web", "--follow"}, wantAmbiguous: true},
		{name: "ps-structured-format", args: []string{"ps", "--format", "{{json .}}"}, wantAmbiguous: true},
		{name: "images-structured-format", args: []string{"images", "--format={{json .}}"}, wantAmbiguous: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !prep.ForcePassthrough {
				t.Fatalf("expected passthrough for %#v, got %#v", tc.args, prep)
			}
			if prep.Ambiguous != tc.wantAmbiguous {
				t.Fatalf("unexpected ambiguity for %#v: got %v want %v", tc.args, prep.Ambiguous, tc.wantAmbiguous)
			}
		})
	}
}

func TestDockerParentPrepareMoveLeadingFlags(t *testing.T) {
	f := NewDockerToolFilter()
	cases := map[string]struct {
		args     []string
		dispatch string
	}{
		"leading context images":    {args: []string{"--context", "bench", "images"}, dispatch: "docker images"},
		"leading context eq ps":     {args: []string{"--context=bench", "ps"}, dispatch: dockerPSDispatch},
		"leading host logs":         {args: []string{"-H", "unix:///var/run/docker.sock", "logs", "api"}, dispatch: "docker logs|container=api"},
		"leading host compose":      {args: []string{"-H", "unix:///var/run/docker.sock", "compose", "logs", "api"}, dispatch: "docker compose logs|scope=api"},
		"leading host compose ps":   {args: []string{"-H", "unix:///var/run/docker.sock", "compose", "ps"}, dispatch: "docker compose ps"},
		"leading host compose file": {args: []string{"-H", "unix:///var/run/docker.sock", "compose", "-f", "compose.yml", "logs", "api"}, dispatch: "docker compose logs|scope=api"},
		"leading config logs":       {args: []string{"--config", "/tmp/docker", "logs", "web"}, dispatch: "docker logs|container=web"},
		"leading log-level ps":      {args: []string{"--log-level", "debug", "ps"}, dispatch: dockerPSDispatch},
		"leading tlsverify ps":      {args: []string{"--tlsverify", "ps"}, dispatch: dockerPSDispatch},
		"leading debug bool":        {args: []string{"--debug", "ps"}, dispatch: dockerPSDispatch},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if prep.ForcePassthrough {
				t.Fatalf("expected routed dispatch for args=%#v got %#v", tc.args, prep)
			}
			if !strings.HasPrefix(prep.DispatchKey, tc.dispatch) {
				t.Fatalf("dispatch mismatch for args=%#v want prefix %q got %q", tc.args, tc.dispatch, prep.DispatchKey)
			}
		})
	}
}

func TestDockerParentPrepareNoArgsUnsupportedAndLogsWithoutContainer(t *testing.T) {
	f := NewDockerToolFilter()
	cases := []struct {
		name string
		args []string
	}{
		{name: "no-args", args: nil},
		{name: "unsupported-subcommand", args: []string{"rm", "container"}},
		{name: "logs-without-container", args: []string{"logs", "--tail", "50"}},
		{name: "compose-logs-unknown-flag", args: []string{"compose", "logs", "--since-ish", "5m"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if prep := f.Prepare(tc.args); !prep.ForcePassthrough {
				t.Fatalf("expected passthrough for %#v, got %#v", tc.args, prep)
			}
		})
	}
}

func TestDockerParentPrepareImagesArgsPreservedWhenNonEmpty(t *testing.T) {
	f := NewDockerToolFilter()
	prep := f.Prepare([]string{"images", "--all"})
	if prep.ForcePassthrough {
		t.Fatalf("expected routed images dispatch, got %#v", prep)
	}
	if prep.DispatchKey != "docker images" {
		t.Fatalf("expected docker images dispatch, got %#v", prep)
	}
	if len(prep.NormalizedArgs) != 2 || prep.NormalizedArgs[0] != "images" || prep.NormalizedArgs[1] != "--all" {
		t.Fatalf("expected args preserved for non-empty images invocation, got %#v", prep.NormalizedArgs)
	}
}

func TestDockerToolProcessDelegatesByDispatchKeyAndFallsBackToNoop(t *testing.T) {
	f := NewDockerToolFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("line-a\n", "a", 1)
	_ = mem.Add("line-b\n", "b", 2)

	flush := f.Process(engine.Event{
		Type:     engine.EventExit,
		Tool:     "docker",
		Dispatch: "docker logs|container=api",
		Stream:   engine.StdoutStream,
	}, mem)
	if flush.Action != engine.ActionFlush || !strings.Contains(flush.Output, "line-a\nline-b\n") {
		t.Fatalf("expected docker logs delegated flush, got %#v", flush)
	}

	noop := f.Process(engine.Event{
		Type:     engine.EventLine,
		Tool:     "docker",
		Dispatch: "",
		Stream:   engine.StdoutStream,
		Line:     "fallback-line\n",
	}, engine.NewOrderedSetBuffer())
	if noop.Action != engine.ActionImmediate || noop.Output != "fallback-line\n" {
		t.Fatalf("expected noop fallback immediate output, got %#v", noop)
	}
	for _, ev := range []engine.EventType{engine.EventTick, engine.EventEOF, engine.EventExit} {
		got := f.Process(engine.Event{
			Type:   ev,
			Tool:   "docker",
			Stream: engine.StdoutStream,
		}, engine.NewOrderedSetBuffer())
		if got.Action != engine.ActionIgnore || got.Output != "" {
			t.Fatalf("expected noop fallback ignore for %v, got %#v", ev, got)
		}
	}
}
