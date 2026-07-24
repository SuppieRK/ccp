package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type stubAdapter struct {
	id        string
	detectDir string
	plan      []PlannedArtifact
}

const (
	agentsFileName = "AGENTS.md"
)

func (s stubAdapter) ID() string {
	return s.id
}

func (s stubAdapter) DetectRoot(scopeRoot string) string {
	if s.detectDir == "" {
		return filepath.Join(scopeRoot, "missing-"+s.id)
	}
	return filepath.Join(scopeRoot, s.detectDir)
}

func (s stubAdapter) Install(_ Context, _ WriterFunc) (InstallResult, error) {
	return InstallResult{}, errors.New("not implemented")
}

func (s stubAdapter) Plan(_ Context) []PlannedArtifact {
	return s.plan
}

func (s stubAdapter) Verify(_ Context) error {
	return nil
}

func writeFileWriter(path string, data []byte, perm os.FileMode) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	old, err := os.ReadFile(path)
	if err == nil && string(old) == string(data) {
		return false, nil
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

type hookRunResult struct {
	stdout   string
	stderr   string
	log      string
	exitCode int
}

type hookTestTB interface {
	Helper()
	TempDir() string
	Fatalf(format string, args ...any)
	Skip(args ...any)
}

func runHookScript(t hookTestTB, scriptName, logName, script, input string, withCmdshape bool) hookRunResult {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook runtime tests are skipped on Windows")
	}

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, scriptName)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}

	pathParts := []string{filepath.Dir(bashPath)}
	if withCmdshape {
		binDir := filepath.Join(tmp, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatalf("mkdir fake bin: %v", err)
		}
		fakeCmdshape := filepath.Join(binDir, "cmdshape")
		if err := os.WriteFile(fakeCmdshape, []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write fake cmdshape: %v", err)
		}
		pathParts = append(pathParts, binDir)
	}

	cmd := exec.Command(bashPath, "./"+scriptName)
	cmd.Dir = tmp
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(),
		"TMPDIR=.",
		"PATH="+bashFriendlyTestPATH(pathParts),
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run hook script: %v stderr=%s", err, stderr.String())
		}
	}

	logPath := filepath.Join(tmp, logName)
	logData, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read hook log: %v", err)
	}
	return hookRunResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		log:      string(logData),
		exitCode: exitCode,
	}
}

func bashFriendlyTestPath(path string) string {
	if runtime.GOOS != "windows" {
		return path
	}
	return filepath.ToSlash(path)
}

func bashFriendlyTestPATH(prefixes []string) string {
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		parts = append(parts, bashFriendlyTestPath(prefix))
	}
	if runtime.GOOS == "windows" {
		return strings.Join(parts, ":")
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

type decodeTestTB interface {
	Helper()
	Fatalf(format string, args ...any)
}

func decodeClaudeHookOutput(t decodeTestTB, raw string) string {
	t.Helper()

	var payload struct {
		HookSpecificOutput struct {
			UpdatedInput struct {
				Command string `json:"command"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode hook output: %v raw=%q", err, raw)
	}
	return payload.HookSpecificOutput.UpdatedInput.Command
}
