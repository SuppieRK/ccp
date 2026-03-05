package cli

import "testing"

const (
	errParseFailedPrefix = "parse failed: %v"
	flagCaptureRaw       = "--capture-raw"
	rawArtifactsDir      = ".artifacts/raw"
)

func TestParseRawModesExecution(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		captureRaw bool
	}{
		{name: "raw", args: []string{"--raw", "ls", "-la"}},
		{name: "capture-raw", args: []string{flagCaptureRaw, "ls", "-la"}, captureRaw: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mustParse(t, tc.args)
			if !opts.Raw {
				t.Fatalf("expected raw mode for args=%#v", tc.args)
			}
			if opts.CaptureRaw != tc.captureRaw {
				t.Fatalf("unexpected capture-raw for args=%#v: got %v want %v", tc.args, opts.CaptureRaw, tc.captureRaw)
			}
			if len(opts.CommandArgs) == 0 || opts.CommandArgs[0] != "ls" {
				t.Fatalf("unexpected command args: %#v", opts.CommandArgs)
			}
		})
	}
}

func TestParseCaptureRawDirExecution(t *testing.T) {
	opts := mustParse(t, []string{flagCaptureRaw, "--capture-raw-dir", rawArtifactsDir, "ls", "-la"})
	if !opts.CaptureRaw {
		t.Fatal("expected capture-raw mode to be enabled")
	}
	if opts.CaptureRawDir != rawArtifactsDir {
		t.Fatalf("unexpected capture-raw-dir: %q", opts.CaptureRawDir)
	}
}

func TestParseRejectsCaptureRawDirWithoutCapture(t *testing.T) {
	assertParseFails(t, []string{"--capture-raw-dir", rawArtifactsDir, "ls"})
}

func TestParseCaptureRawWithConfidential(t *testing.T) {
	opts := mustParse(t, []string{flagCaptureRaw, "--confidential", "com.foo, org.acme ,com.foo", "ls"})
	if len(opts.ConfidentialRedactions) != 2 {
		t.Fatalf("unexpected confidential redactions: %#v", opts.ConfidentialRedactions)
	}
	if opts.ConfidentialRedactions[0] != "com.foo" || opts.ConfidentialRedactions[1] != "org.acme" {
		t.Fatalf("unexpected confidential values: %#v", opts.ConfidentialRedactions)
	}
}

func TestParseRejectsConfidentialWithoutCaptureRaw(t *testing.T) {
	assertParseFails(t, []string{"--confidential", "com.foo", "ls"})
}

func TestParseRejectsRawForLifecycleCommands(t *testing.T) {
	assertLifecycleCommandsRejectFlag(t, "--raw")
}

func TestParseRejectsCaptureRawForLifecycleCommands(t *testing.T) {
	assertLifecycleCommandsRejectFlag(t, flagCaptureRaw)
}

func assertLifecycleCommandsRejectFlag(t *testing.T, flag string) {
	t.Helper()
	cases := []string{"init", "gain", "history", "upgrade", "uninstall"}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			assertParseFails(t, []string{flag, cmd})
		})
	}
}

func TestParseRejectsVerbosityFlags(t *testing.T) {
	cases := []string{"-v", "-vv", "-vvv"}
	for _, flag := range cases {
		t.Run(flag, func(t *testing.T) {
			assertParseFails(t, []string{flag, "ls"})
		})
	}
}

func TestParseDebugAndStrictFlags(t *testing.T) {
	opts := mustParse(t, []string{"--debug-filter", "--strict", "ls"})
	if !opts.DebugFilter {
		t.Fatal("expected debug-filter mode to be enabled")
	}
	if !opts.Strict {
		t.Fatal("expected strict mode to be enabled")
	}
}

func TestParseHelpFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "long", args: []string{"--help"}},
		{name: "short", args: []string{"-h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mustParse(t, tc.args)
			if !opts.ShowHelp {
				t.Fatalf("expected help mode to be enabled for args=%#v", tc.args)
			}
			if len(opts.CommandArgs) != 0 {
				t.Fatalf("expected no command args for help, got %#v", opts.CommandArgs)
			}
		})
	}
}

func TestParseHelpBypassesRawLifecycleValidation(t *testing.T) {
	opts := mustParse(t, []string{"--help", "--raw", "init"})
	if !opts.ShowHelp || !opts.Raw {
		t.Fatalf("unexpected parsed options: %#v", opts)
	}
}

func assertParseFails(t *testing.T, args []string) {
	t.Helper()
	if _, err := Parse(args); err == nil {
		t.Fatalf("expected parse error for args=%#v", args)
	}
}

func mustParse(t *testing.T, args []string) Options {
	t.Helper()
	opts, err := Parse(args)
	if err != nil {
		t.Fatalf(errParseFailedPrefix, err)
	}
	return opts
}
