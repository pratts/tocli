#!/usr/bin/env bash
# Usage: git-why-record.sh [path]
#
# Coverage check, not a lookup: walks commit history from newest to oldest
# and reports every commit newer than the most recent one that already has a
# reasoning note attached -- "the gap" since notes were last kept up to date.
#
# That count is the tail gap only, not total coverage: un-noted commits older
# than the most recent note (e.g. gaps left behind a mid-history backfill)
# aren't counted, and the output says so, so the number isn't misread as
# "total commits without notes" (#1).
#
# Purely mechanical: no AI, no note generation, no side effects. Answers "are
# we behind on notes?" -- distinct from git-why.sh ("what's the reasoning for
# this file/line?") and git-notes-grep.sh ("which commit has a note about
# this concept?").
#
# With no argument, walks the whole current branch. Pass a path to scope the
# walk to commits touching that file/directory instead.
#
# Notes are read via `git log`'s own %N format placeholder, joined with each
# commit in the same walk -- no separate `git notes list` call, no bash-side
# set-building. The walk stops at the first noted commit it finds, so on a
# repo with no notes fetched at all, this still reads the full history (there
# is nothing to stop early on), but does it in one `git log` process rather
# than forking git once per commit.

set -euo pipefail

path="${1:-}"
max_print=20

US=$'\x1f' # field separator between sha / subject / note
RS=$'\x1e' # record separator between commits

log_args=(--format="%H${US}%s${US}%N${RS}")
if [[ -n "$path" ]]; then
  log_args+=(-- "$path")
fi

any=0
gap=()
last_noted=""
last_noted_line=""
while IFS= read -r -d "$RS" record; do
  record="${record#$'\n'}" # strip the newline git appends after each record
  sha="${record%%"$US"*}"
  rest="${record#*"$US"}"
  subject="${rest%%"$US"*}"
  note="${rest#*"$US"}"
  note="${note%$'\n'}" # %N appends one trailing newline when a note exists

  any=1
  if [[ -n "$note" ]]; then
    last_noted="$sha"
    last_noted_line="${sha:0:9} -- ${subject}"
    break
  fi
  gap+=("${sha:0:9}  ${subject}")
done < <(git log "${log_args[@]}")

if [[ "$any" -eq 0 ]]; then
  echo "no commit history found${path:+ for $path}"
  exit 0
fi

if [[ -z "$last_noted" ]]; then
  echo "no notes found in this repo's history -- nothing to compare against"
  exit 0
fi

if [[ ${#gap[@]} -eq 0 ]]; then
  echo "up to date -- the newest commit already has a reasoning note"
  echo "0 commits since the last note"
  exit 0
fi

printed=0
for entry in "${gap[@]}"; do
  if [[ "$printed" -ge "$max_print" ]]; then
    break
  fi
  echo "$entry"
  printed=$((printed + 1))
done

total=${#gap[@]}
if [[ "$total" -gt "$max_print" ]]; then
  echo "... and $((total - max_print)) more not shown"
fi

echo
echo "$total commits since the last note (last noted: $last_noted_line)"
echo "tail only -- un-noted commits older than that note aren't counted"
