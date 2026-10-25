#!/usr/bin/env bash
# Usage: git-notes-grep.sh <keyword> [more keywords...]
#
# Searches the text of every commit's reasoning note for the given
# keyword(s) (case-insensitive, OR'd together, matched literally -- regex
# metacharacters in a keyword are escaped, not interpreted) and prints
# matching commits, newest first.
#
# Recall-first lookup: use this when you remember a concept ("locking",
# "port exhaustion", "reboot") but not which file or commit it lives in.
# Complements git-why.sh, which is navigation-first (you already know the
# file/line and want the reasoning attached to it).
#
# Notes are read via `git log`'s own %N format placeholder, joined with each
# commit in the same walk, already in newest-first order -- no separate
# `git notes list` call, no accumulating/sorting match arrays.

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: git-notes-grep.sh <keyword> [more keywords...]" >&2
  exit 1
fi

# Escape ERE metacharacters so keywords match literally. No `declare -A` /
# `mapfile` here on purpose -- see the note on 8af5907 (macOS ships bash 3.2,
# which has neither).
escape_regex() {
  local s="$1" out="" c i
  for (( i = 0; i < ${#s}; i++ )); do
    c="${s:i:1}"
    case "$c" in
      '.'|'*'|'['|']'|'^'|'$'|'+'|'?'|'('|')'|'{'|'}'|'|'|'\')
        out+="\\$c" ;;
      *)
        out+="$c" ;;
    esac
  done
  printf '%s' "$out"
}

pattern=""
for kw in "$@"; do
  escaped=$(escape_regex "$kw")
  pattern="${pattern:+${pattern}|}${escaped}"
done

US=$'\x1f' # field separator between sha / subject / note
RS=$'\x1e' # record separator between commits

found=0
while IFS= read -r -d "$RS" record; do
  record="${record#$'\n'}" # strip the newline git appends after each record
  sha="${record%%"$US"*}"
  rest="${record#*"$US"}"
  subject="${rest%%"$US"*}"
  note="${rest#*"$US"}"
  note="${note%$'\n'}" # %N appends one trailing newline when a note exists

  [[ -z "$note" ]] && continue
  if grep -qiE "$pattern" <<<"$note"; then
    found=1
    echo "commit ${sha:0:9} -- $subject"
    echo "$note" | sed 's/^/  /'
    echo
  fi
done < <(git log --format="%H${US}%s${US}%N${RS}")

if [[ "$found" -eq 0 ]]; then
  echo "no notes matched: $*"
fi
