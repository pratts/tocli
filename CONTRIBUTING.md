# Contributing to tocli

## Getting started

```bash
git clone https://github.com/pratts/tocli.git
cd tocli
go build -o tocli ./cmd/tocli
go test ./... -race
```

Requires the Go version pinned in `go.mod` or newer. Platform support is
macOS and Linux only (see the README) — there's no point trying to get this
working on Windows without first tackling `internal/process`'s reliance on
POSIX signals, `flock`, and `setsid`.

## Before opening a PR

CI (`.github/workflows/ci.yml`) runs on every push and PR and will reject:

```bash
go build ./...
go vet ./...
go test ./... -race -timeout 180s
gofmt -l .          # must print nothing
go mod tidy && git diff --exit-code go.mod go.sum   # must be a no-op
```

Run all five yourself before pushing — it's the same thing CI will tell you,
just faster to find out locally. `internal/process` has real
platform-specific files (`bootid_linux.go` vs `bootid_darwin.go`); CI runs
the test matrix on both Linux and macOS runners specifically so each one is
actually exercised, not just compiled.

## The one architectural rule that matters

**`internal/tui` is a presentation layer only.** No TUI model may construct
a `torrent.Client`, touch `~/.tocli` directly, or otherwise re-implement
anything `internal/engine` already does. Every TUI screen and every plain
CLI command call the same `internal/engine` functions — `StartTorrent`,
`PauseTorrent`, `ResumeTorrent`, `RemoveTorrent`, `OpenPreview` — so there is
exactly one implementation of what each action does. If you find yourself
writing logic in a TUI model that isn't "call an existing engine/store/
process function and render the result," that logic belongs in
`internal/engine` (or `internal/store`/`internal/process`, if it's really
about on-disk state or process lifecycle), not in `internal/tui`.

The package boundaries this implies, roughly bottom-up:

- `internal/process` — process lifecycle (spawn, signal, liveness probe,
  boot id, advisory lock). No dependency on anything else in this project.
- `internal/store` — on-disk state (`config.json`/`state.json`, atomic
  writes, id derivation, liveness reconciliation). Depends on `process`.
- `internal/config` — global `~/.tocli/config.toml`. Depends on `store`.
- `internal/engine` — wraps `anacrolix/torrent` and orchestrates
  start/pause/resume/remove. Depends on `store`, `config`, `process`.
- `internal/cli` and `internal/tui` — both depend on `engine`/`store`/
  `config`/`process`, never the other way around, and never duplicate what
  those packages already do.

## Code conventions

- Wrap errors with `%w` and enough context to know what was being attempted
  (`fmt.Errorf("load torrent %s: %w", id, err)`), and don't swallow one
  silently unless there's a comment saying why it's safe to (e.g. a missed
  `state.json` write shouldn't abort an otherwise-healthy download — but say
  that, don't just drop it).
- Any write to a file under `~/.tocli` goes through the atomic
  write-temp-then-rename helper in `internal/store` (`writeJSONAtomic`), not
  a bare `os.WriteFile` — a reader (`tocli list` running concurrently with a
  background download process) must never see a half-written file.
- User-supplied values that become filesystem paths (a torrent id, in
  particular) must be validated before use — see `store.ValidateID` and how
  `store.TorrentDir` calls it. Don't add a new path helper that bypasses it.
- A blocking wait on network I/O (anywhere `t.GotInfo()` or similar is
  awaited) takes a `context.Context` and is bounded by it — see
  `engine.MetadataTimeout` and `engine.waitForInfo`. Don't add a new bare
  blocking wait; a dead swarm has hung this tool for real in the past.
- Comments explain *why*, not *what* — skip a comment if removing it
  wouldn't confuse a future reader; write one for a non-obvious constraint,
  a subtle invariant, or a workaround for specific library behavior.

## Tests

Prefer testing at the level a bug would actually be caught: `internal/store`
and `internal/engine` have plenty of tests that exercise real subprocesses,
real file locks, and real (if synthetic/offline) torrents rather than
mocking everything — that's deliberate, since this project's hardest bugs
have been process-lifecycle and concurrency issues that mocks tend to hide.
If you're fixing a bug, add a test that would have caught it before the fix,
not just one that exercises the fixed code path.
