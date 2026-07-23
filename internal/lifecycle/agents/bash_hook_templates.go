package agents

import (
	"fmt"
	"os"
	"strings"
)

func hookAgentTitle(agent string) string {
	if agent == "" {
		return ""
	}
	return strings.ToUpper(agent[:1]) + agent[1:]
}

func verifyBashRewriteHook(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(raw)
	for _, snippet := range []string{
		"tokenize_segment()",
		"is_shell_builtin_or_keyword()",
		"consume_env_prefix()",
		"consume_sudo_prefix()",
	} {
		if !strings.Contains(content, snippet) {
			return fmt.Errorf("hook script lacks conservative command classification: %s", path)
		}
	}
	return nil
}

func bashJSONHelpers() string {
	return `
trim_whitespace() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

json_extract_string() {
  local json="$1"
  shift
  local key marker rest char out escape
  for key in "$@"; do
    marker="\"$key\""
    [[ "$json" == *"$marker"* ]] || continue
    rest="${json#*"$marker"}"
    [[ "$rest" =~ ^[[:space:]]*:[[:space:]]*\" ]] || continue
    rest="${rest#*:}"
    while [[ "$rest" =~ ^[[:space:]] ]]; do
      rest="${rest:1}"
    done
    [[ "${rest:0:1}" == '"' ]] || continue
    rest="${rest:1}"
    out=""
    escape=0
    while ((${#rest} > 0)); do
      char="${rest:0:1}"
      rest="${rest:1}"
      if (( escape )); then
        case "$char" in
          n) out+=$'\n' ;;
          r) out+=$'\r' ;;
          t) out+=$'\t' ;;
          '"') out+="\"" ;;
          "\\") out+="\\" ;;
          /) out+="/" ;;
          *) out+="$char" ;;
        esac
        escape=0
        continue
      fi
      case "$char" in
        "\\") escape=1 ;;
        '"') printf '%s' "$out"; return 0 ;;
        *) out+="$char" ;;
      esac
    done
  done
  return 1
}

json_escape() {
  local raw="$1"
  local i char out=""
  for ((i = 0; i < ${#raw}; i++)); do
    char="${raw:i:1}"
    case "$char" in
      "\\") out+="\\\\" ;;
      '"') out+="\\\"" ;;
      $'\n') out+="\\n" ;;
      $'\r') out+="\\r" ;;
      $'\t') out+="\\t" ;;
      *) out+="$char" ;;
    esac
  done
  printf '%s' "$out"
}
`
}

func bashRewriteHelpers() string {
	return `
is_shell_builtin_or_keyword() {
  case "$1" in
    .|:|\[|alias|bg|bind|break|builtin|caller|cd|command|compgen|complete|compopt|continue|coproc|declare|dirs|disown|echo|enable|eval|exec|exit|export|false|fc|fg|getopts|hash|help|history|jobs|kill|let|local|logout|mapfile|popd|printf|pushd|pwd|read|readarray|readonly|return|set|shift|shopt|source|suspend|test|times|trap|true|type|typeset|ulimit|umask|unalias|unset|wait|case|do|done|elif|else|esac|fi|for|function|if|in|select|then|time|until|while|\{|\}) return 0 ;;
  esac
  return 1
}

tokenize_segment() {
  local input="$1" i=0 len=${#1} char
  local token="" start=0 started=0 in_single=0 in_double=0 escape=0
  TOKENS=()
  TOKEN_STARTS=()
  while (( i < len )); do
    char="${input:i:1}"
    if (( escape )); then
      token+="$char"
      escape=0
      ((i++))
      continue
    fi
    if (( in_single )); then
      if [[ "$char" == "'" ]]; then
        in_single=0
      else
        token+="$char"
      fi
      ((i++))
      continue
    fi
    if (( in_double )); then
      case "$char" in
        "\\") escape=1 ;;
        '"') in_double=0 ;;
        '$'|$'\x60') return 1 ;;
        *) token+="$char" ;;
      esac
      ((i++))
      continue
    fi
    if [[ "$char" =~ [[:space:]] ]]; then
      if (( started )); then
        TOKENS+=("$token")
        TOKEN_STARTS+=("$start")
        token=""
        started=0
      fi
      ((i++))
      continue
    fi
    if (( ! started )); then
      start=$i
      started=1
    fi
    case "$char" in
      "\\") escape=1 ;;
      "'") in_single=1 ;;
      '"') in_double=1 ;;
      '$'|$'\x60'|'<'|'>'|'|'|'&'|';'|'('|')'|'{'|'}'|'#') return 1 ;;
      *) token+="$char" ;;
    esac
    ((i++))
  done
  (( escape == 0 && in_single == 0 && in_double == 0 )) || return 1
  if (( started )); then
    TOKENS+=("$token")
    TOKEN_STARTS+=("$start")
  fi
  ((${#TOKENS[@]} > 0))
}

is_assignment_token() {
  [[ "$1" =~ ^[a-zA-Z_][a-zA-Z0-9_]*= ]]
}

consume_env_prefix() {
  local count=${#TOKENS[@]}
  ((TOKEN_INDEX++))
  while (( TOKEN_INDEX < count )); do
    case "${TOKENS[TOKEN_INDEX]}" in
      --) ((TOKEN_INDEX++)); return 0 ;;
      -u|--unset|-C|--chdir|--argv0)
        (( TOKEN_INDEX + 1 < count )) || return 1
        ((TOKEN_INDEX+=2))
        ;;
      --unset=*|--chdir=*|--argv0=*|-i|--ignore-environment|-0|--null)
        ((TOKEN_INDEX++))
        ;;
      -*)
        return 1
        ;;
      *)
        if is_assignment_token "${TOKENS[TOKEN_INDEX]}"; then
          ((TOKEN_INDEX++))
        else
          return 0
        fi
        ;;
    esac
  done
  return 1
}

consume_sudo_prefix() {
  local count=${#TOKENS[@]}
  ((TOKEN_INDEX++))
  while (( TOKEN_INDEX < count )); do
    case "${TOKENS[TOKEN_INDEX]}" in
      --) ((TOKEN_INDEX++)); return 0 ;;
      -u|-g|-h|-p|-C|-T|-R|-D|-r|-t|--user|--group|--host|--prompt|--close-from|--command-timeout|--chroot|--chdir|--role|--type)
        (( TOKEN_INDEX + 1 < count )) || return 1
        ((TOKEN_INDEX+=2))
        ;;
      --user=*|--group=*|--host=*|--prompt=*|--close-from=*|--command-timeout=*|--chroot=*|--chdir=*|--role=*|--type=*|-A|-b|-E|-e|-H|-K|-k|-n|-P|-S|-V|-v|-l|-i|-s)
        ((TOKEN_INDEX++))
        ;;
      -*)
        return 1
        ;;
      *)
        return 0
        ;;
    esac
  done
  return 1
}

rewrite_segment() {
  local segment="$1" count command token start i
  tokenize_segment "$segment" || return 1
  count=${#TOKENS[@]}
  TOKEN_INDEX=0
  while (( TOKEN_INDEX < count )) && is_assignment_token "${TOKENS[TOKEN_INDEX]}"; do
    ((TOKEN_INDEX++))
  done
  while (( TOKEN_INDEX < count )); do
    case "${TOKENS[TOKEN_INDEX]}" in
      env) consume_env_prefix || return 1 ;;
      sudo) consume_sudo_prefix || return 1 ;;
      *) break ;;
    esac
    while (( TOKEN_INDEX < count )) && is_assignment_token "${TOKENS[TOKEN_INDEX]}"; do
      ((TOKEN_INDEX++))
    done
  done
  (( TOKEN_INDEX < count )) || return 1
  command="${TOKENS[TOKEN_INDEX]}"
  if [[ "$command" == "ccp" ]]; then
    printf '%s' "$segment"
    return 0
  fi
  is_shell_builtin_or_keyword "$command" && return 1
  case "$command" in
    xargs) return 1 ;;
    sh|bash|dash|zsh|ksh)
      for token in "${TOKENS[@]:TOKEN_INDEX+1}"; do
        [[ "$token" == "-c" || "$token" == "-lc" ]] && return 1
      done
      ;;
    find)
      for token in "${TOKENS[@]:TOKEN_INDEX+1}"; do
        [[ "$token" == "-exec" || "$token" == "-execdir" || "$token" == "-ok" || "$token" == "-okdir" ]] && return 1
      done
      ;;
  esac
  start=${TOKEN_STARTS[TOKEN_INDEX]}
  printf '%sccp %s' "${segment:0:start}" "${segment:start}"
}

rewrite_command() {
  local input="$1"
  local out="" segment="" piece
  local i=0 len=${#input}
  local char next pair
  local in_single=0 in_double=0 escape=0

  while (( i < len )); do
    char="${input:i:1}"
    next="${input:i+1:1}"
    pair="${input:i:2}"

    if (( escape )); then
      segment+="$char"
      escape=0
      ((i++))
      continue
    fi
    if (( in_single )); then
      segment+="$char"
      [[ "$char" == "'" ]] && in_single=0
      ((i++))
      continue
    fi
    if (( in_double )); then
      segment+="$char"
      case "$char" in
        "\\") escape=1 ;;
        '"') in_double=0 ;;
        '$'|$'\x60') printf '%s' "$input"; return 0 ;;
      esac
      ((i++))
      continue
    fi

    case "$char" in
      "\\")
        segment+="$char"
        escape=1
        ((i++))
        continue
        ;;
      "'")
        segment+="$char"
        in_single=1
        ((i++))
        continue
        ;;
      '"')
        segment+="$char"
        in_double=1
        ((i++))
        continue
        ;;
      '$'|$'\x60'|'<'|'>'|'('|')'|'{'|'}'|'#'|$'\n'|$'\r')
        printf '%s' "$input"
        return 0
        ;;
    esac

    if [[ "$pair" == "&&" || "$pair" == "||" ]]; then
      segment="${segment%"${segment##*[![:space:]]}"}"
      piece="$(rewrite_segment "$segment")" || { printf '%s' "$input"; return 0; }
      [[ -n "$out" ]] && out+=" "
      out+="$piece $pair"
      segment=""
      ((i+=2))
      while [[ "${input:i:1}" =~ [[:space:]] ]]; do
        ((i++))
      done
      continue
    fi
    if [[ "$char" == "|" || "$char" == "&" ]]; then
      printf '%s' "$input"
      return 0
    fi
    if [[ "$char" == ";" ]]; then
      segment="${segment%"${segment##*[![:space:]]}"}"
      piece="$(rewrite_segment "$segment")" || { printf '%s' "$input"; return 0; }
      [[ -n "$out" ]] && out+=" "
      out+="$piece $char"
      segment=""
      ((i++))
      while [[ "${input:i:1}" =~ [[:space:]] ]]; do
        ((i++))
      done
      continue
    fi

    segment+="$char"
    ((i++))
  done

  (( escape == 0 && in_single == 0 && in_double == 0 )) || { printf '%s' "$input"; return 0; }
  piece="$(rewrite_segment "$segment")" || { printf '%s' "$input"; return 0; }
  [[ -n "$out" ]] && out+=" "
  out+="$piece"
  printf '%s' "$out"
}
`
}

func bashRewriteHookScriptContent(agent, logFile string) string {
	return "#!/bin/bash\n" +
		"# generated by ccp init for " + agent + "\n" +
		"# " + hookAgentTitle(agent) + " PreToolUse hook: rewrite shell commands to use ccp.\n\n" +
		"LOG_FILE=\"${TMPDIR:-/tmp}/" + logFile + "\"\n\n" +
		"log_hook() {\n  printf '%s\\n' \"$1\" >>\"$LOG_FILE\"\n}\n" +
		bashJSONHelpers() +
		bashRewriteHelpers() +
		`
if ! command -v ccp >/dev/null 2>&1; then
  log_hook "skip-no-ccp"
  exit 0
fi

INPUT="$(</dev/stdin)"
if [[ -z "$INPUT" ]]; then
  log_hook "skip-empty-input"
  exit 0
fi

CMD="$(json_extract_string "$INPUT" command)"
if [[ -z "$CMD" ]]; then
  log_hook "skip-no-command"
  exit 0
fi

REWRITTEN_CMD="$(rewrite_command "$CMD")"
if [[ -z "$REWRITTEN_CMD" ]]; then
  log_hook "skip-empty-rewrite"
  exit 0
fi
if [[ "$REWRITTEN_CMD" == "$CMD" ]]; then
  log_hook "skip-no-change"
  exit 0
fi
if ! bash -n -c "$REWRITTEN_CMD" >/dev/null 2>&1; then
  log_hook "skip-invalid-shell"
  exit 0
fi

ESCAPED_CMD="$(json_escape "$REWRITTEN_CMD")"
printf '%s\n' "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"allow\",\"permissionDecisionReason\":\"ccp auto-rewrite\",\"updatedInput\":{\"command\":\"$ESCAPED_CMD\"}}}"
`
}
