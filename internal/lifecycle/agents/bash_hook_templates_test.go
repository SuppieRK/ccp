package agents

import (
	"strings"
	"testing"
)

func TestHookScriptsUseOnlyBashAndCCPRuntime(t *testing.T) {
	scripts := map[string]string{
		"claude":    claudeHookScriptContent(),
		"continue":  continueHookScriptContent(),
		"codebuddy": codebuddyHookScriptContent(),
		"cline":     clineHookScriptContent(),
		"windsurf":  windsurfHookScriptContent(),
	}
	for name, script := range scripts {
		if !strings.HasPrefix(script, "#!/bin/bash\n") {
			t.Fatalf("%s hook should use bash shebang, got: %s", name, script)
		}
		for _, forbidden := range []string{"jq", "awk", "grep", "cat", "sed", "/usr/bin/env"} {
			if strings.Contains(script, forbidden) {
				t.Fatalf("%s hook should not depend on %q, got: %s", name, forbidden, script)
			}
		}
	}
}
