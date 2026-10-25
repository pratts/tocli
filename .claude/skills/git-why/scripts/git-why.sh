#!/usr/bin/env bash
# Usage: git-why.sh <file> [line]
#
# Finds the commit(s) that touched a file (or one specific line, if given)
# and prints the reasoning note attached to each one, if any.
#
# git log/blame answer "what changed and when" -- this answers "why".
#
# Notes are read via `git log`'s own %N format placeholder, which joins each
# commit with its note (empty if none) in the same process that walks
# history -- no separate `git notes list`/`git notes show` call, no bash-side
# matching. %N follows core.notesRef the same way `git notes show` does, so
# behavior doesn't diverge for a repo using a non-default notes ref.

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: git-why.sh <file> [line]" >&2
  exit 1
fi

file="$1"
line="${2:-}"

US=$'\x1f' # field separator between sha / subject / note
RS=$'\x1e' # record separator between commits

if [[ -n "$line" ]]; then
  sha=$(git blame -L "${line},${line}" --porcelain -- "$file" 2>/dev/null | head -1 | cut -d' ' -f1)
  if [[ -z "$sha" ]]; then
    echo "could not blame ${file}:${line} -- check the file and line number" >&2
    exit 1
  fi

  info=$(git log -1 --format="%s${US}%N" "$sha")
  subject="${info%%"$US"*}"
  note="${info#*"$US"}"
  note="${note%$'\n'}" # %N appends one trailing newline when a note exists

  found=0
  if [[ -n "$note" ]]; then
    found=1
    echo "commit ${sha:0:9} -- $subject"
    echo "$note" | sed 's/^/  /'
    echo
  fi

  if [[ "$found" -eq 0 ]]; then
    echo "no reasoning note found for ${file}:${line}"
  fi
else
  any=0
  found=0
  while IFS= read -r -d "$RS" record; do
    record="${record#$'\n'}" # strip the newline git appends after each record
    sha="${record%%"$US"*}"
    rest="${record#*"$US"}"
    subject="${rest%%"$US"*}"
    note="${rest#*"$US"}"
    note="${note%$'\n'}"

    any=1
    if [[ -n "$note" ]]; then
      found=1
      echo "commit ${sha:0:9} -- $subject"
      echo "$note" | sed 's/^/  /'
      echo
    fi
  done < <(git log --format="%H${US}%s${US}%N${RS}" -- "$file")

  if [[ "$any" -eq 0 ]]; then
    echo "no history found for $file -- check the path" >&2
    exit 1
  fi

  if [[ "$found" -eq 0 ]]; then
    echo "no reasoning note found for $file"
  fi
fi
