# git-why

A Claude Code skill that captures the design reasoning behind non-trivial
commits as git notes attached to the commit itself. The "why" — rejected
alternatives, constraints that forced an approach, tradeoffs — becomes
retrievable from the terminal forever, independent of the chat that produced
it.

## Install

Clone this repo into `.claude/skills/git-why/` inside the target project,
then remove the nested `.git`:

```bash
git clone <this-repo-url> .claude/skills/git-why && rm -rf .claude/skills/git-why/.git
```

Keep the two commands joined with `&&`. If the clone fails (no network, or
`.claude/skills/git-why` already exists), `rm -rf` must not run: on an
existing clone it would delete that clone's history, including any
unpushed work in it.

The `rm -rf` step matters if the target project is itself a git repo (the
usual case): without it, `git add` there treats `.claude/skills/git-why` as
an embedded repository and stages a bare gitlink instead of the actual
files — SKILL.md and the scripts silently don't get committed, and the next
person to clone the target project gets an empty directory where the skill
should be, with no error at any point. Deleting `.git` first makes it plain
vendored files that commit normally.

The resulting path **must** be `.claude/skills/git-why/SKILL.md`. A common
way this silently fails to load is an extra level of nesting, e.g.
`.claude/skills/git-why/git-why/SKILL.md` — double check the clone didn't
create a nested directory.

### Updating

With `.git` removed there's nothing to `git pull`. To update, delete the
folder and install again:

```bash
rm -rf .claude/skills/git-why && git clone <this-repo-url> .claude/skills/git-why && rm -rf .claude/skills/git-why/.git
```

This replaces the folder wholesale, so any local edits to the skill are
lost. Review `git diff .claude/skills/git-why` in the target project before
committing the update.

### Optional: the commit-reminder hook

The skill's own `.claude/settings.json` is **not** read when it's installed
under `.claude/skills/` — Claude Code only loads a project's top-level
`.claude/settings.json`. To get the reminder after every commit in the
target project, add this to that file (merging into any existing `hooks`):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PROJECT_DIR}/.claude/skills/git-why/.claude/hooks/git-commit-reminder.sh"
          }
        ]
      }
    ]
  }
}
```

Pointing at the vendored script, rather than copying it, means updating the
skill also updates the hook. Leave out an `if` condition: the script decides
for itself whether the command was a commit (see below), and
`"if": "Bash(git commit *)"` would hide commits like `git -C dir commit`
from it. The script needs `jq` to read the command; without it, it reminds
after every Bash call rather than never.

## How it works

Claude applies this skill proactively: right after a commit that involved
real design reasoning, and whenever you ask "why does this exist" about
existing code. This only happens when Claude is present in the session —
there's no hook or background automation by default, and no retroactive
notes for commits made outside Claude. The full criteria for what warrants a
note, how to write one, and where to put it when a decision spans two files
lives in [SKILL.md](SKILL.md) — that's the canonical source Claude actually
follows; this file just covers install and the lookup tools.

This repo also ships an optional `PostToolUse` hook
(`.claude/settings.json` and `.claude/hooks/git-commit-reminder.sh`), which
fires that reminder deterministically after every `git commit` instead of
relying on Claude to remember. It's active when this repo is the open
project; other projects opt in as described under
[Install](#optional-the-commit-reminder-hook). The script itself checks
whether a Bash command ran `git commit`, including forms like
`git -C dir commit`, `git -c key=value commit` and `git --no-pager commit`,
while ignoring look-alikes such as `git log --grep=commit` or
`git commit-tree`.

## Usage

### `scripts/git-why.sh <file> [line]`

Find the commit(s) that touched a file (or one line), and print any
reasoning note attached to them. Navigation-first: use it when you already
know the file.

```bash
scripts/git-why.sh internal/process/lock.go
scripts/git-why.sh internal/process/lock.go 41
```

### `scripts/git-notes-grep.sh <keyword> [more keywords...]`

Search every note's text for a keyword (case-insensitive, OR'd together,
matched literally). Recall-first: use it when you remember a concept but not
which file it lives in.

```bash
scripts/git-notes-grep.sh lock reboot
```

### `scripts/git-why-record.sh [path]`

Coverage check: report which recent commits don't have a reasoning note yet.
Purely mechanical, no AI — just how many commits have piled up since the
last note. It counts the tail only: un-noted commits older than the most
recent note aren't included, so it isn't a total-coverage number.

```bash
scripts/git-why-record.sh
scripts/git-why-record.sh internal/process/lock.go
```

## Caveat: notes don't travel with push/fetch/clone by default

`git notes` live on a separate ref, `refs/notes/commits`, which is **not**
included in a normal `git push`, `git fetch`, or `git clone`.

For a solo project this doesn't matter. But the moment a project is shared
or cloned onto another machine, notes must be explicitly pushed and fetched,
or they will silently not show up for a collaborator:

```bash
git push origin refs/notes/commits
git fetch origin refs/notes/commits:refs/notes/commits
```

This is the single most likely thing to confuse someone the first time
notes don't appear after a clone — if you're setting this up on a shared
repo, make sure everyone knows to run the fetch command above.

## License

MIT — see [LICENSE](LICENSE).
