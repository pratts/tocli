#!/usr/bin/env bash
# PostToolUse hook (matcher: Bash, if: Bash(git commit *)).
#
# PostToolUse only fires when the matched tool call succeeded -- a failing
# Bash command (e.g. a commit rejected by a pre-commit hook) fires
# PostToolUseFailure instead, which this hook is not registered on. So by the
# time this script runs, the commit has already succeeded; no exit-code
# checking is needed here.
#
# This script makes no judgment call about whether the commit deserves a
# note -- it only guarantees the reminder fires every time, deterministically.
# The decision of whether to actually write one stays with Claude, per
# SKILL.md's own skip criteria (mechanical changes don't need a note).

cat >/dev/null # drain stdin

cat <<'JSON'
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "A git commit just succeeded. Per this repo's SKILL.md (installed elsewhere as .claude/skills/git-why/SKILL.md): if the commit involved non-obvious design reasoning -- a rejected alternative, a constraint that ruled out a simpler approach, a subtle bug fix, a deliberate tradeoff -- it should get a git notes entry, following the skill's note-writing guidance. Mechanical commits (formatting, a dependency bump, a typo fix) do not need one."
  }
}
JSON
