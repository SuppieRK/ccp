package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type jsPluginVerifyRequirement struct {
	Snippet string
	Msg     string
}

type ManagedJSPluginAdapterSpec struct {
	ID                 ID
	DetectRootPath     string
	ConfigDirName      string
	MissingFileFmt     string
	VerifyRequirements []jsPluginVerifyRequirement
}

type ManagedJSPluginAdapter struct {
	spec ManagedJSPluginAdapterSpec
}

const managedJSPluginFileName = "cmdshape-rewrite.js"

var openCodeJSPluginSpec = ManagedJSPluginAdapterSpec{
	ID:             AgentOpenCode,
	DetectRootPath: ".opencode",
	ConfigDirName:  "opencode",
	MissingFileFmt: "missing opencode plugin file: %s",
	VerifyRequirements: []jsPluginVerifyRequirement{
		{Snippet: `"tool.execute.before"`, Msg: "opencode plugin missing tool.execute.before hook: %s"},
		{Snippet: `input.tool !== "bash"`, Msg: "opencode plugin missing bash-only guard: %s"},
	},
}

func NewManagedJSPluginAdapter(spec ManagedJSPluginAdapterSpec) ManagedJSPluginAdapter {
	return ManagedJSPluginAdapter{spec: spec}
}

func (a ManagedJSPluginAdapter) ID() string { return string(a.spec.ID) }

func (a ManagedJSPluginAdapter) DetectRoot(scopeRoot string) string {
	return ResolveRepoScopedPath(scopeRoot, a.spec.DetectRootPath)
}

func (a ManagedJSPluginAdapter) Plan(ctx Context) []PlannedArtifact {
	return []PlannedArtifact{{
		Kind:    ArtifactSettings,
		Path:    managedJSPluginPath(ctx, a.spec.ConfigDirName),
		Content: managedBashRewritePluginContent(),
		Perm:    0o644,
	}}
}

func (a ManagedJSPluginAdapter) Install(ctx Context, write WriterFunc) (InstallResult, error) {
	return InstallPlannedArtifacts(a.Plan(ctx), write)
}

func (a ManagedJSPluginAdapter) Verify(ctx Context) error {
	pluginPath := managedJSPluginPath(ctx, a.spec.ConfigDirName)
	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf(a.spec.MissingFileFmt, pluginPath)
	}
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		return err
	}
	content := string(data)
	requirements := append([]jsPluginVerifyRequirement{
		{Snippet: `function rewriteCommand(input)`, Msg: "managed plugin missing conservative command classifier: %s"},
		{Snippet: `shellBuiltinsAndKeywords`, Msg: "managed plugin missing builtin guard: %s"},
	}, a.spec.VerifyRequirements...)
	for _, req := range requirements {
		if !strings.Contains(content, req.Snippet) {
			return fmt.Errorf(req.Msg, pluginPath)
		}
	}
	return nil
}

func (a ManagedJSPluginAdapter) Uninstall(ctx Context) (InstallResult, error) {
	pluginPath := managedJSPluginPath(ctx, a.spec.ConfigDirName)
	removed, err := removeFileIfExists(pluginPath)
	if err != nil {
		return InstallResult{}, err
	}
	if removed {
		return InstallResult{Applied: 1}, nil
	}
	return InstallResult{Noop: 1}, nil
}

func managedJSPluginPath(ctx Context, configDirName string) string {
	return filepath.Join(managedJSPluginConfigRoot(ctx, configDirName), "plugins", managedJSPluginFileName)
}

func managedJSPluginConfigRoot(ctx Context, configDirName string) string {
	if strings.TrimSpace(ctx.HomeDir) != "" {
		return filepath.Join(ctx.HomeDir, ".config", configDirName)
	}
	return filepath.Join(ctx.ScopeRoot, "."+configDirName)
}

func managedBashRewritePluginContent() string {
	return `const shellBuiltinsAndKeywords = new Set([
  ".", ":", "[", "alias", "bg", "bind", "break", "builtin", "caller", "cd",
  "command", "compgen", "complete", "compopt", "continue", "coproc", "declare",
  "dirs", "disown", "echo", "enable", "eval", "exec", "exit", "export", "false",
  "fc", "fg", "getopts", "hash", "help", "history", "jobs", "kill", "let",
  "local", "logout", "mapfile", "popd", "printf", "pushd", "pwd", "read",
  "readarray", "readonly", "return", "set", "shift", "shopt", "source", "suspend",
  "test", "times", "trap", "true", "type", "typeset", "ulimit", "umask",
  "unalias", "unset", "wait", "case", "do", "done", "elif", "else", "esac",
  "fi", "for", "function", "if", "in", "select", "then", "time", "until",
  "while", "{", "}",
]);

function assignmentToken(token) {
  return /^[A-Za-z_][A-Za-z0-9_]*=/.test(token);
}

function tokenizeSegment(input) {
  const tokens = [];
  const starts = [];
  let token = "";
  let start = 0;
  let started = false;
  let single = false;
  let double = false;
  let escape = false;
  for (let i = 0; i < input.length; i += 1) {
    const char = input[i];
    if (escape) {
      token += char;
      escape = false;
      continue;
    }
    if (single) {
      if (char === "'") single = false;
      else token += char;
      continue;
    }
    if (double) {
      if (char === "\\") escape = true;
      else if (char === '"') double = false;
      else if (char === "$" || char.charCodeAt(0) === 96) return null;
      else token += char;
      continue;
    }
    if (/\s/.test(char)) {
      if (started) {
        tokens.push(token);
        starts.push(start);
        token = "";
        started = false;
      }
      continue;
    }
    if (!started) {
      start = i;
      started = true;
    }
    if (char === "\\") escape = true;
    else if (char === "'") single = true;
    else if (char === '"') double = true;
    else if ("$<>|&;(){}#".includes(char) || char.charCodeAt(0) === 96) return null;
    else token += char;
  }
  if (escape || single || double) return null;
  if (started) {
    tokens.push(token);
    starts.push(start);
  }
  return tokens.length === 0 ? null : { tokens, starts };
}

function consumeEnv(tokens, index) {
  index += 1;
  while (index < tokens.length) {
    const token = tokens[index];
    if (token === "--") return index + 1;
    if (["-u", "--unset", "-C", "--chdir", "--argv0"].includes(token)) {
      if (index + 1 >= tokens.length) return -1;
      index += 2;
    } else if (/^(--unset=|--chdir=|--argv0=)/.test(token) ||
      ["-i", "--ignore-environment", "-0", "--null"].includes(token)) {
      index += 1;
    } else if (token.startsWith("-")) {
      return -1;
    } else if (assignmentToken(token)) {
      index += 1;
    } else {
      return index;
    }
  }
  return -1;
}

function consumeSudo(tokens, index) {
  const consuming = new Set([
    "-u", "-g", "-h", "-p", "-C", "-T", "-R", "-D", "-r", "-t", "--user",
    "--group", "--host", "--prompt", "--close-from", "--command-timeout",
    "--chroot", "--chdir", "--role", "--type",
  ]);
  const standalone = new Set([
    "-A", "-b", "-E", "-e", "-H", "-K", "-k", "-n", "-P", "-S", "-V", "-v",
    "-l", "-i", "-s",
  ]);
  index += 1;
  while (index < tokens.length) {
    const token = tokens[index];
    if (token === "--") return index + 1;
    if (consuming.has(token)) {
      if (index + 1 >= tokens.length) return -1;
      index += 2;
    } else if (/^--(user|group|host|prompt|close-from|command-timeout|chroot|chdir|role|type)=/.test(token) ||
      standalone.has(token)) {
      index += 1;
    } else if (token.startsWith("-")) {
      return -1;
    } else {
      return index;
    }
  }
  return -1;
}

function rewriteSegment(segment) {
  const parsed = tokenizeSegment(segment);
  if (!parsed) return null;
  const { tokens, starts } = parsed;
  let index = 0;
  while (index < tokens.length && assignmentToken(tokens[index])) index += 1;
  while (index < tokens.length) {
    if (tokens[index] === "env") index = consumeEnv(tokens, index);
    else if (tokens[index] === "sudo") index = consumeSudo(tokens, index);
    else break;
    if (index < 0) return null;
    while (index < tokens.length && assignmentToken(tokens[index])) index += 1;
  }
  if (index >= tokens.length) return null;
  const command = tokens[index];
  if (command === "cmdshape") return segment;
  if (shellBuiltinsAndKeywords.has(command) || command === "xargs") return null;
  const tail = tokens.slice(index + 1);
  if (["sh", "bash", "dash", "zsh", "ksh"].includes(command) &&
      tail.some((token) => token === "-c" || token === "-lc")) return null;
  if (command === "find" &&
      tail.some((token) => ["-exec", "-execdir", "-ok", "-okdir"].includes(token))) return null;
  const start = starts[index];
  return segment.slice(0, start) + "cmdshape " + segment.slice(start);
}

function rewriteCommand(input) {
  let output = "";
  let segment = "";
  let single = false;
  let double = false;
  let escape = false;
  for (let i = 0; i < input.length; i += 1) {
    const char = input[i];
    const pair = input.slice(i, i + 2);
    if (escape) {
      segment += char;
      escape = false;
      continue;
    }
    if (single) {
      segment += char;
      if (char === "'") single = false;
      continue;
    }
    if (double) {
      segment += char;
      if (char === "\\") escape = true;
      else if (char === '"') double = false;
      else if (char === "$" || char.charCodeAt(0) === 96) return null;
      continue;
    }
    if (char === "\\") {
      segment += char;
      escape = true;
      continue;
    }
    if (char === "'") {
      segment += char;
      single = true;
      continue;
    }
    if (char === '"') {
      segment += char;
      double = true;
      continue;
    }
    if ("$<>(){}#\n\r".includes(char) || char.charCodeAt(0) === 96) return null;
    if (pair === "&&" || pair === "||") {
      segment = segment.replace(/\s+$/, "");
      const rewritten = rewriteSegment(segment);
      if (rewritten === null) return null;
      output += (output ? " " : "") + rewritten + " " + pair;
      segment = "";
      i += 1;
      while (i + 1 < input.length && /\s/.test(input[i + 1])) i += 1;
      continue;
    }
    if (char === "|" || char === "&") return null;
    if (char === ";") {
      segment = segment.replace(/\s+$/, "");
      const rewritten = rewriteSegment(segment);
      if (rewritten === null) return null;
      output += (output ? " " : "") + rewritten + " ;";
      segment = "";
      while (i + 1 < input.length && /\s/.test(input[i + 1])) i += 1;
      continue;
    }
    segment += char;
  }
  if (escape || single || double) return null;
  const rewritten = rewriteSegment(segment);
  if (rewritten === null) return null;
  return output + (output ? " " : "") + rewritten;
}

export default async function cmdshapeRewritePlugin() {
  return {
    "tool.execute.before": async (input, output) => {
      if (input.tool !== "bash") {
        return;
      }
      const command = output?.args?.command;
      if (typeof command !== "string") {
        return;
      }
      const rewritten = rewriteCommand(command);
      if (rewritten === null || rewritten === command) {
        return;
      }
      output.args = output.args || {};
      output.args.command = rewritten;
    },
  };
}
`
}
