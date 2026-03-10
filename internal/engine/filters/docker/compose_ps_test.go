package dockerfilters

import (
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

func TestComposePSCompactionPreservesServiceIdentity(t *testing.T) {
	raw := strings.Join([]string{
		"NAME                 IMAGE          COMMAND         SERVICE   CREATED         STATUS                     PORTS",
		"demo-api-1           demo-api       \"./api\"        api       10 seconds ago  Up 10 seconds             0.0.0.0:8080->8080/tcp",
		"demo-worker-1        demo-worker    \"./worker\"     worker    8 seconds ago   Exited (1) 1 second ago  -",
	}, "\n") + "\n"

	out, ok := compactComposePS(raw, 15)
	if !ok {
		t.Fatal("expected compact docker compose ps output")
	}
	if strings.Contains(out, "docker compose ps:") {
		t.Fatalf("expected no compose ps summary, got:\n%s", out)
	}
	if !strings.Contains(out, "[ok] demo-api-1 service=api image=demo-api status=Up 10 seconds") {
		t.Fatalf("expected healthy service row, got:\n%s", out)
	}
	if !strings.Contains(out, "[!] demo-worker-1 service=worker image=demo-worker status=Exited (1) 1 second ago") {
		t.Fatalf("expected non-healthy service row, got:\n%s", out)
	}
}

func TestComposePSCompactionShortensImageAndOmitsEmptyPorts(t *testing.T) {
	raw := strings.Join([]string{
		"NAME                 IMAGE                         COMMAND         SERVICE   CREATED         STATUS                     PORTS",
		"demo-worker-1        ghcr.io/acme/demo-worker:1   \"./worker\"     worker    8 seconds ago   Exited (1) 1 second ago  -",
	}, "\n") + "\n"
	out, ok := compactComposePS(raw, 15)
	if !ok {
		t.Fatal("expected compact docker compose ps output")
	}
	if !strings.Contains(out, "image=demo-worker:1") {
		t.Fatalf("expected shortened image, got:\n%s", out)
	}
	if strings.Contains(out, "ports=-") {
		t.Fatalf("expected empty ports omitted, got:\n%s", out)
	}
}

func TestComposePSHeaderMismatchFallsBack(t *testing.T) {
	raw := "SOMETHING ELSE\nfoo bar baz\n"
	if _, ok := compactComposePS(raw, 15); ok {
		t.Fatalf("expected header mismatch fallback")
	}
}

func TestComposePSCompactionBoundedRowRendering(t *testing.T) {
	raw := strings.Join([]string{
		"NAME        IMAGE     COMMAND   SERVICE   CREATED   STATUS      PORTS",
		"demo-a-1    img-a     \"a\"      api       now       running     8080->80/tcp",
		"demo-b-1    img-b     \"b\"      worker    now       exited (1)  -",
		"demo-c-1    img-c     \"c\"      web       now       running     9090->90/tcp",
	}, "\n") + "\n"
	out, ok := compactComposePS(raw, 1)
	if !ok {
		t.Fatal("expected compact docker compose ps output")
	}
	if !strings.Contains(out, "... +2 more") {
		t.Fatalf("expected bounded overflow marker, got:\n%s", out)
	}
}

func TestComposePSProcessRuntimeCollectionAndFlushParity(t *testing.T) {
	f := NewComposePSFilter()
	mem := engine.NewOrderedSetBuffer()
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout line collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventTick, Stream: engine.StdoutStream}, mem); got.Action != engine.ActionCollect {
		t.Fatalf("expected stdout tick collect, got %#v", got)
	}
	if got := f.Process(engine.Event{Type: engine.EventLine, Stream: engine.StderrStream, Line: "compose error\n"}, mem); got.Action != engine.ActionImmediate {
		t.Fatalf("expected stderr immediate, got %#v", got)
	}
}
