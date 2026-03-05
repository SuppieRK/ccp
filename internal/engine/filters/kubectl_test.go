package filters

import (
	"slices"
	"strings"
	"testing"

	"go-command-compression-proxy/internal/engine"
)

const (
	kubectlGetPodsCmd       = "kubectl get pods"
	kubectlGetServicesCmd   = "kubectl get services"
	kubectlLogsCmd          = "kubectl logs"
	kubectlBenchNamespace   = "ccp-bench"
	kubectlNamespaceEqBench = "--namespace=ccp-bench"
	kubectlContextFlag      = "--context"
	kubectlKindBenchContext = "kind-bench"
)

func TestKubectlParentPrepareDispatchesToSubcommand(t *testing.T) {
	f := NewKubectlToolFilter()
	prep := f.Prepare([]string{"get", "pods", "-o", "wide"})
	if prep.ForcePassthrough {
		t.Fatal("expected foldable get pods to stay in kubectl filter")
	}
	if prep.DispatchKey != kubectlGetPodsCmd {
		t.Fatalf("expected dispatch kubectl get pods, got %q", prep.DispatchKey)
	}
}

func TestKubectlParentMetadataAndNoArgsPassthrough(t *testing.T) {
	f := NewKubectlToolFilter()
	if f.Tool() != "kubectl" {
		t.Fatalf("expected kubectl tool, got %q", f.Tool())
	}
	if !slices.Equal(f.Aliases(), []string{"kubectl.exe"}) {
		t.Fatalf("unexpected kubectl aliases: %#v", f.Aliases())
	}
	prep := f.Prepare(nil)
	if !prep.ForcePassthrough {
		t.Fatalf("expected no-args passthrough, got %#v", prep)
	}
	if prep.DispatchKey != "" {
		t.Fatalf("expected empty dispatch on no-args passthrough, got %q", prep.DispatchKey)
	}
}

func TestKubectlParentPreparePassthroughCases(t *testing.T) {
	f := NewKubectlToolFilter()
	cases := []struct {
		name string
		args []string
	}{
		{name: "structured-jsonpath", args: []string{"get", "pods", "-o", "jsonpath={.items[*].metadata.name}"}},
		{name: "structured-output-equals", args: []string{"get", "pods", "--output=name"}},
		{name: "logs-follow", args: []string{"logs", "pod-1", "-f"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep := f.Prepare(tc.args)
			if !prep.ForcePassthrough {
				t.Fatalf("expected passthrough for %v, got %#v", tc.args, prep)
			}
		})
	}
}

func TestKubectlGetPodsHeaderGateFallback(t *testing.T) {
	f := NewKubectlToolFilter()
	mem := engine.NewOrderedSetBuffer()
	_ = mem.Add("NOM NOM\n", "NOM NOM\n", 1)
	_ = mem.Add("a b c\n", "a b c\n", 2)
	d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "kubectl", Dispatch: kubectlGetPodsCmd, Stream: engine.StdoutStream}, mem)
	if d.Action != engine.ActionFlush {
		t.Fatalf("expected flush, got %q", d.Action)
	}
	if d.Output != "NOM NOM\na b c\n" {
		t.Fatalf("expected passthrough on unknown header, got %q", d.Output)
	}
}

func TestKubectlGetPodsCases(t *testing.T) {
	f := NewKubectlToolFilter()
	cases := []struct {
		name         string
		lines        []string
		wantContains []string
	}{
		{
			name: "anomaly-and-healthy-folding",
			lines: []string{
				"NAME READY STATUS RESTARTS AGE\n",
				"a 1/1 Running 0 10m\n",
				"b 1/1 Running 0 9m\n",
				"c 0/1 CrashLoopBackOff 3 1m\n",
			},
			wantContains: []string{"CrashLoopBackOff", "[2] pods: 1/1 Running 0"},
		},
		{
			name: "all-namespaces-grouping",
			lines: []string{
				"NAMESPACE NAME READY STATUS RESTARTS AGE\n",
				"kube-system a 1/1 Running 0 1m\n",
				"kube-system b 1/1 Running 0 1m\n",
				"default c 1/1 Running 0 1m\n",
			},
			wantContains: []string{"kube-system: [2 pods: all Running]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mem := engine.NewOrderedSetBuffer()
			for i, line := range tc.lines {
				_ = mem.Add(line, line, uint64(i+1))
			}
			d := f.Process(engine.Event{Type: engine.EventEOF, Tool: "kubectl", Dispatch: kubectlGetPodsCmd, Stream: engine.StdoutStream}, mem)
			for _, want := range tc.wantContains {
				if !strings.Contains(d.Output, want) {
					t.Fatalf("expected output to contain %q, got:\n%s", want, d.Output)
				}
			}
		})
	}
}

func TestKubectlStderrIsolationAndNoResourceMessage(t *testing.T) {
	f := NewKubectlToolFilter()
	d := f.Process(engine.Event{Type: engine.EventLine, Tool: "kubectl", Dispatch: kubectlGetPodsCmd, Stream: engine.StderrStream, Line: "No resources found in default namespace.\n"}, engine.NewOrderedSetBuffer())
	if d.Action != engine.ActionImmediate {
		t.Fatalf("expected immediate stderr emission, got %q", d.Action)
	}
	if strings.TrimSpace(d.Output) != "No pods found" {
		t.Fatalf("expected concise no-resource message, got %q", d.Output)
	}
}

func TestResolveKubectlSubcommandFromArgsMoveLeadingFlags(t *testing.T) {
	reg, err := buildKubectlSubcommandRegistry()
	if err != nil {
		t.Fatalf("build kubectl subcommand registry: %v", err)
	}
	cases := map[string]struct {
		args         []string
		wantDispatch string
		wantSubArgs  []string
	}{
		"get pods plain": {
			args:         []string{"get", "pods"},
			wantDispatch: kubectlGetPodsCmd,
			wantSubArgs:  []string{},
		},
		"get pod singular": {
			args:         []string{"get", "pod"},
			wantDispatch: kubectlGetPodsCmd,
			wantSubArgs:  []string{},
		},
		"get services": {
			args:         []string{"get", "services"},
			wantDispatch: kubectlGetServicesCmd,
			wantSubArgs:  []string{},
		},
		"get svc alias": {
			args:         []string{"get", "svc"},
			wantDispatch: kubectlGetServicesCmd,
			wantSubArgs:  []string{},
		},
		"get nodes": {
			args:         []string{"get", "nodes"},
			wantDispatch: "kubectl get nodes",
			wantSubArgs:  []string{},
		},
		"logs plain": {
			args:         []string{"logs", "pod-a"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a"},
		},
		"logs with tail": {
			args:         []string{"logs", "pod-a", "--tail", "50"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", "--tail", "50"},
		},
		"logs follow long flag passthrough": {
			args: []string{"logs", "pod-a", "--follow"},
		},
		"logs follow short flag passthrough": {
			args: []string{"logs", "pod-a", "-f"},
		},
		"leading namespace then logs": {
			args:         []string{"-n", kubectlBenchNamespace, "logs", "pod-a"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", "-n", kubectlBenchNamespace},
		},
		"leading namespace eq then logs": {
			args:         []string{kubectlNamespaceEqBench, "logs", "pod-a"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", kubectlNamespaceEqBench},
		},
		"leading context namespace then logs": {
			args:         []string{kubectlContextFlag, kubectlKindBenchContext, "-n", kubectlBenchNamespace, "logs", "pod-a"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", kubectlContextFlag, kubectlKindBenchContext, "-n", kubectlBenchNamespace},
		},
		"leading context eq namespace then logs": {
			args:         []string{"--context=" + kubectlKindBenchContext, "-n", kubectlBenchNamespace, "logs", "pod-a"},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", "--context=" + kubectlKindBenchContext, "-n", kubectlBenchNamespace},
		},
		"leading namespace then get pods": {
			args:         []string{"-n", kubectlBenchNamespace, "get", "pods"},
			wantDispatch: kubectlGetPodsCmd,
			wantSubArgs:  []string{"-n", kubectlBenchNamespace},
		},
		"leading context namespace then get pods all ns": {
			args:         []string{kubectlContextFlag, kubectlKindBenchContext, "-n", kubectlBenchNamespace, "get", "pods", "-A"},
			wantDispatch: kubectlGetPodsCmd,
			wantSubArgs:  []string{"-A", kubectlContextFlag, kubectlKindBenchContext, "-n", kubectlBenchNamespace},
		},
		"leading namespace eq then get services": {
			args:         []string{kubectlNamespaceEqBench, "get", "services"},
			wantDispatch: kubectlGetServicesCmd,
			wantSubArgs:  []string{kubectlNamespaceEqBench},
		},
		"leading unknown flag passthrough": {
			args: []string{"--bad-global", "logs", "pod-a"},
		},
		"leading server user cluster then get nodes": {
			args:         []string{"--server", "https://x", "--user", "dev", "--cluster", "c1", "get", "nodes"},
			wantDispatch: "kubectl get nodes",
			wantSubArgs:  []string{"--server", "https://x", "--user", "dev", "--cluster", "c1"},
		},
		"double dash passthrough": {
			args: []string{"--", "get", "pods"},
		},
		"non-supported subcommand passthrough": {
			args: []string{"describe", "pod", "x"},
		},
		"missing subcommand after globals passthrough": {
			args: []string{"-n", kubectlBenchNamespace},
		},
		"resource unknown passthrough": {
			args: []string{"get", "deployments"},
		},
		"logs pod namespace trailing": {
			args:         []string{"logs", "pod-a", "-n", kubectlBenchNamespace},
			wantDispatch: kubectlLogsCmd,
			wantSubArgs:  []string{"pod-a", "-n", kubectlBenchNamespace},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assertResolvedKubectlSubcommand(t, reg, tc.args, tc.wantDispatch, tc.wantSubArgs)
		})
	}
}

func assertResolvedKubectlSubcommand(t *testing.T, reg *engine.ToolFilterRegistry, args []string, wantDispatch string, wantSubArgs []string) {
	t.Helper()
	f, subArgs := resolveKubectlSubcommandFromArgs(reg, args)
	if wantDispatch == "" {
		if f != nil {
			t.Fatalf("expected no dispatch, got %q", f.Tool())
		}
		return
	}
	if f == nil {
		t.Fatalf("expected dispatch %q, got nil filter", wantDispatch)
	}
	if got := f.Tool(); got != wantDispatch {
		t.Fatalf("dispatch mismatch: want %q got %q", wantDispatch, got)
	}
	if !slices.Equal(subArgs, wantSubArgs) {
		t.Fatalf("sub-args mismatch: want %#v got %#v", wantSubArgs, subArgs)
	}
}
