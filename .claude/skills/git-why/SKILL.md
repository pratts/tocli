---
name: git-why
description: Capture the design reasoning behind non-trivial commits as git notes attached to the commit itself, so "why" survives long after the chat that produced it is gone. Use this proactively right after any git commit that involved a real design decision — a rejected alternative, a constraint that forced an approach, a tradeoff, a subtle bug fix — not just a summary of what changed. Also use this whenever the user asks "why does this exist", "why was this done this way", or "what was the reasoning behind X" about existing code — look up the answer with the bundled scripts before falling back to searching old chat history.
---

# Commit Reasoning Notes

Git commit messages capture *what* changed. They rarely capture *why* — and when an
agent makes a design decision (rejects an alternative, works around a library
limitation, picks one tradeoff over another), that reasoning usually only exists in
the chat that produced it. Once the chat is gone, so is the reasoning. This skill
attaches that reasoning directly to the commit, using `git notes`, so it's
retrievable from the terminal forever, independent of any chat history.

## When to write a note

After making a commit, write a note if — and only if — the change involved
non-obvious reasoning. Good candidates:

- An approach was chosen specifically because a simpler one didn't work (a library
  limitation, a race condition, a platform constraint).
- An alternative was seriously considered and rejected — worth recording *why* it
  was rejected, not just that it was.
- A subtle bug fix where the root cause isn't obvious from the diff alone.
- A tradeoff was made deliberately (e.g. simplicity over performance, or vice versa).

Skip it for mechanical changes: dependency bumps, formatting, straightforward
additions with no real decision behind them, typo fixes. Not every commit needs a
note — a note on every commit is noise, and noise gets ignored.

## How to write a good note

Do NOT restate the commit message or summarize the diff — that information is
already in `git log`. A note should answer the question someone will ask while
staring at this code later: **"why is it built this way, and not some other way?"**

Keep it to a few sentences. Structure, loosely:

1. The constraint or problem that ruled out the obvious/simpler approach.
2. What was chosen instead, and why it addresses that constraint.
3. (If relevant) what was considered and rejected, in one line — not a full
   discussion.

Write the note with:

```bash
git notes add -m "<reasoning text>" <commit-sha>
```

If a note already exists on that commit and you need to add to it rather than
replace it:

```bash
git notes append -m "<additional reasoning>" <commit-sha>
```

### Example

Commit message: `Add per-torrent advisory lock to prevent spawn race`

Good note:
> Needed to prevent two processes racing to download into the same directory if the
> parent dies between spawning the child and recording its PID. Considered checking
> PID + boot_id alone, but that only detects the race after it's already happened.
> flock is auto-released on any process death (including SIGKILL), so it closes the
> race window itself rather than just detecting it afterward.

Bad note (just restates the commit): "Added a lock file to prevent two processes
from running at once."

### Decisions spanning two files: definition vs. call site

Some decisions involve both a primitive's definition (a lock, a helper function)
and a specific call site that uses it in a particular way. Put the note on the
commit that actually makes the decision — usually the integration/call-site
commit, since the primitive itself is often decision-agnostic (it doesn't know
how it'll be used, only the caller does).

But the primitive's own file is often the first place someone looks when they ask
"why does this exist" — they're staring at the lock, not the caller. If that's a
likely lookup point, add a short pointer note there too:

```bash
git notes add -m "See commit <sha> for why this is used this way in <caller>." <primitive-sha>
```

That way a file-scoped lookup on the primitive doesn't silently come up empty — it
at least points the reader to where the real reasoning lives, instead of relying
on them to already know to check the caller's commit.

## How to look up existing reasoning

Don't rely on memory or chat history to answer "why does this exist" — use the
bundled scripts, which read directly from git.

**Not sure where the reasoning might be attached — start with `scripts/git-notes-grep.sh`:**

```bash
scripts/git-notes-grep.sh lock reboot
```

Searches the text of every note in the repo for the given keyword(s)
(case-insensitive, OR'd together, matched literally — special characters in a
keyword are treated as plain text, not regex) and prints the matching commits
and their notes — regardless of which file each commit touched. This matters because a note
often lives on the commit that makes a decision, not on every file that decision
affects: e.g. "why we call X this way" may be attached to the commit that adds the
*call site* in Y, not to any commit that touched X's own file. A file-scoped
lookup on X can come up empty even though a perfectly good answer exists
elsewhere. When it's unclear whether the reasoning might live on a caller's commit
rather than the definition's own file, try this first or alongside
`git-why.sh` — not only as a fallback after `git-why.sh` comes up empty.

**You already have a good guess which commit/file the reasoning lives on — use `scripts/git-why.sh`:**

```bash
scripts/git-why.sh internal/process/lock.go
scripts/git-why.sh internal/process/lock.go 41
```

Finds the commit(s) that touched that file (or that exact line, if given), and
prints any reasoning note attached to them. This is precise but strictly
file-scoped — it will only ever surface notes on commits that touched the exact
path you query — so treat it as the tool for once you know where to look, not as
a general-purpose "why does this exist" search on its own.

Prefer these over re-reading old chat transcripts — the notes are the
purpose-built, structured source of truth for "why," and they stay accurate even
after the chat that produced them is long gone.

**Checking whether notes are falling behind — use `scripts/git-why-record.sh`:**

```bash
scripts/git-why-record.sh
scripts/git-why-record.sh internal/process/lock.go
```

Both scripts above answer "what's the reasoning for X" once a note exists.
This one answers a different question — "are we behind on writing notes at
all" — by reporting every commit newer than the most recent one that already
has a note attached. It's a purely mechanical coverage check (no AI, no note
generation, no side effects), useful for spot-checking that the habit is
actually being kept up. Takes an optional path argument to scope the check
to one file/directory's history instead of the whole branch. The count is
the tail gap since the newest note, not total coverage — un-noted commits
older than that note aren't counted, so a mid-history backfill can make the
number drop sharply without anything older being fixed.

## Known limitation

`git notes` live on a separate ref (`refs/notes/commits`) and are **not** included
in a normal `git push` / `git fetch` / `git clone`. For a solo repo this doesn't
matter. If the repo is ever shared or cloned onto another machine, notes need to be
explicitly pushed/fetched:

```bash
git push origin refs/notes/commits
git fetch origin refs/notes/commits:refs/notes/commits
```

Mention this to the user if the project moves from solo to shared, since notes
silently not showing up for a collaborator is a confusing failure mode.
