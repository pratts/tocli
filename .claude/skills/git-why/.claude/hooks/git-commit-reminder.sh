#!/usr/bin/env bash
# PostToolUse hook (matcher: Bash).
#
# PostToolUse only fires when the matched tool call succeeded -- a failing
# Bash command (e.g. a commit rejected by a pre-commit hook) fires
# PostToolUseFailure instead, which this hook is not registered on. So by the
# time this script runs, any commit in the command has already succeeded; no
# exit-code checking is needed here.
#
# "Was this a git commit" is decided here rather than by an `if` matcher on
# the settings entry. `Bash(git commit *)` needs `git` and `commit` to be
# adjacent, so it misses `git -C dir commit`, `git -c k=v commit` and
# `git --no-pager commit` (#2); loosening it to `Bash(git *commit*)` would
# trade that for false positives like `git log --grep=commit`. Instead, for
# each shell command segment, this skips leading VAR=value assignments,
# requires the word `git`, skips git's own options (and the values of the
# ones that take one), and checks that the subcommand is exactly `commit`.
#
# This script makes no judgment call about whether the commit deserves a
# note -- it only guarantees the reminder fires every time, deterministically.
# The decision of whether to actually write one stays with Claude, per
# SKILL.md's own skip criteria (mechanical changes don't need a note).

input=$(cat)

is_commit() {
	local segment w i
	local -a words
	# One segment per ; & | or newline -- crude, but quoting-aware parsing
	# isn't needed to find a command word at the start of a segment.
	while IFS= read -r segment; do
		read -ra words <<<"$segment"
		i=0
		while [ "$i" -lt "${#words[@]}" ] && [[ ${words[$i]} == [A-Za-z_]*=* ]]; do
			i=$((i + 1))
		done
		[ "${words[$i]:-}" = git ] || continue
		i=$((i + 1))
		while [ "$i" -lt "${#words[@]}" ]; do
			w=${words[$i]}
			case $w in
			-C | -c | --git-dir | --work-tree | --namespace | --exec-path | --config-env | --super-prefix)
				i=$((i + 2)) ;;
			-*)
				i=$((i + 1)) ;;
			*)
				break ;;
			esac
		done
		[ "${words[$i]:-}" = commit ] && return 0
	done < <(tr ';&|' '\n\n\n' <<<"$1")
	return 1
}

# Without jq the command can't be extracted reliably, so fail open (remind)
# rather than silently dropping the reminder this hook exists to guarantee.
if command -v jq >/dev/null 2>&1; then
	cmd=$(jq -r '.tool_input.command // empty' <<<"$input" 2>/dev/null) || cmd=
	is_commit "$cmd" || exit 0
fi

cat <<'JSON'
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "A git commit just succeeded. Check it against the git-why skill's SKILL.md note criteria; skip if mechanical."
  }
}
JSON
