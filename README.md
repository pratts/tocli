# tocli

A terminal BitTorrent client with a process-per-torrent architecture. Every
command works two ways: as a plain, scriptable one-shot invocation, or as an
interactive terminal UI when you're missing an argument or ask for one
explicitly.

```
tocli start ./ubuntu.iso.torrent      # scriptable
tocli start                           # interactive: prompts for a source,
                                       # shows a live preview, then starts
tocli list                            # live dashboard on a terminal,
                                       # a static table when piped/scripted
```

> **Platform support:** macOS and Linux only. `internal/process` relies on
> POSIX signals, `flock`, and `setsid` for the process-per-torrent design;
> there is no Windows build.

## Installing

### `go install`

```bash
go install github.com/pratts/tocli/cmd/tocli@latest
```

Installs the binary to `$(go env GOPATH)/bin` (make sure that's on your
`PATH`). Pin a specific version instead of `@latest` with a released tag,
e.g. `@v1.0.0`.

### Pre-built binaries

Every tag pushed as `vX.Y.Z` is built and published automatically (see
[Releasing](#releasing) below) — grab the `.zip` for your platform from the
[Releases page](https://github.com/pratts/tocli/releases), unzip it, and put
the `tocli` binary on your `PATH`.

## Building from source

Requires the Go version pinned in `go.mod` (currently 1.27) or newer.

```bash
git clone https://github.com/pratts/tocli.git
cd tocli
go build -o tocli ./cmd/tocli
```

This produces a single `tocli` binary with no other runtime dependencies.

## Running

### Commands

| Command | Direct form | What it does |
|---|---|---|
| `tocli start <file-or-magnet>` | resolves metadata, confirms, spawns a background download | `--tui`/`-i` forces the interactive preview; `--yes`/`-y` skips confirmation entirely |
| `tocli list` | prints a table of tracked torrents | `--json` for structured output, `--plain` to force the table on a terminal |
| `tocli pause <id>` | stops a running torrent's background process | `--tui`/`-i` opens a picker instead of taking an id |
| `tocli resume <id>` | respawns a paused/stopped/crashed torrent from cached metadata | same `--tui`/`-i` |
| `tocli remove <id> [--with-data]` | stops it (if running) and forgets it; `--with-data` also deletes the downloaded files | same `--tui`/`-i` |

### Dual-mode rule

If a command has everything it needs to act (an id, a source path), it just
does it — no prompts, safe to script or cron. If it's missing something it
needs, it opens the relevant interactive screen instead of printing a usage
error:

- `tocli start` with no source → a screen to type in a path or magnet link.
- `tocli pause`/`resume`/`remove` with no id → a picker of the relevant
  torrents (only running ones for pause, only resumable ones for resume, all
  of them for remove).
- `tocli start <source>` on a real terminal shows the same interactive
  preview (file tree, live peer count, tracker list) rather than the old
  plain yes/no prompt — pass `--yes` to skip straight through for scripts.
- `tocli list` on a real terminal opens a live, refreshing dashboard where
  you can pause/resume/remove the selected torrent directly; piped or
  non-terminal invocations (and `--plain`/`--json`) always get the static,
  scriptable output.

Every `--tui`/`-i` flag forces the interactive screen even when enough
arguments were given directly.

## Architecture

### Process-per-torrent, not a daemon

There is no long-running tocli server process. `tocli start`/`resume` spawn
one detached background process per torrent (internally, `tocli __run <id>`,
a hidden subcommand), which owns that torrent's `torrent.Client` for its
entire download. `tocli pause` is just `SIGTERM` to that process's pid; the
process's own signal handler closes the torrent client cleanly and marks
itself paused before exiting. There's no IPC between the CLI and the
background processes — they only communicate through files on disk.

This means a crash in one torrent's process can't affect any other torrent,
and there's nothing to keep running when you're not actively downloading
anything.

### On-disk state (`~/.tocli/`)

```
~/.tocli/
  config.toml                 # base download dir, speed limits, port range
  torrents/<id>/
    metainfo.torrent          # cached resolved .torrent bytes
    config.json               # id, name, save path, status, pid, boot id
    state.json                # live progress: percent, rate, peers
    log.txt                   # the background process's stdout/stderr
    lock                      # advisory lock (see below)
```

`<id>` is derived from the torrent's info hash, so re-adding the same
torrent resolves to the same id instead of creating a duplicate.

Because a background process can die without any chance to update its own
bookkeeping (`kill -9`, an OOM kill, the machine losing power), tocli never
fully trusts a stale `status: running`:

- it cross-checks the recorded pid against a real, live process, and
- compares a recorded boot-session id against the current one, so a pid
  that's since been reused by an unrelated process after a reboot is never
  mistaken for the original torrent, and
- each background process holds an OS advisory lock (`flock`) on its
  torrent's `lock` file for as long as it's alive. Unlike the pid/boot-id
  check above, the lock is a direct kernel guarantee rather than an
  inference — it's released automatically the instant the process's file
  descriptors close, for any reason at all — so a second process can never
  start downloading into the same directory while another one already is,
  even in the narrow window right after a crash.

### Package layout

```
cmd/tocli/            entrypoint
internal/
  cli/                cobra command definitions; dual-mode dispatch between
                       the plain path and the TUI
  engine/              wraps anacrolix/torrent: metadata resolution, the
                       background download loop, and the start/pause/
                       resume/remove orchestration shared by every caller
  store/               on-disk state: config.json/state.json (atomic writes),
                       directory layout, id derivation, liveness reconciliation
  process/             process lifecycle: spawning a detached child, signals,
                       the liveness probe, boot-id tracking, the advisory lock
  config/              global ~/.tocli/config.toml
  humanize/            byte/rate formatting for display
  tui/                 the interactive layer (see below)
```

### The TUI is presentation only

`internal/tui` (dashboard, add-flow, and the shared picker/confirm/menu
components in `internal/tui/actions`) never talks to a `torrent.Client` or
writes to `~/.tocli` directly. Every screen calls the exact same
`internal/engine` functions — `StartTorrent`, `PauseTorrent`,
`ResumeTorrent`, `RemoveTorrent` — that the plain CLI commands call, so
there's exactly one implementation of what each action does; the two
layers only differ in when they call it and how they render the result.

## Releasing

Pushing a tag matching `v*` (e.g. `v1.2.3`) triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which
cross-compiles `tocli` for linux/amd64, linux/arm64, darwin/amd64, and
darwin/arm64, zips each binary up with the README and license, generates a
`checksums.txt`, and publishes (or creates, if it doesn't exist yet) a
GitHub Release with all of them attached:

```bash
git tag v1.2.3
git push origin v1.2.3
```

Creating a release through the GitHub UI/CLI on a *new* tag name triggers
the same workflow, since that also pushes the tag. Publishing a release
against a tag that was already pushed earlier does not re-run the build —
the tag push is what triggers it.

## Built with

- [github.com/anacrolix/torrent](https://github.com/anacrolix/torrent) —
  the BitTorrent protocol implementation tocli's engine wraps: metainfo
  parsing, peer/DHT/tracker discovery, and piece downloading.
- [github.com/spf13/cobra](https://github.com/spf13/cobra) — the CLI
  command framework `internal/cli` is built on.
- [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) —
  the terminal UI framework `internal/tui` is built on, along with its
  companion libraries [bubbles](https://github.com/charmbracelet/bubbles)
  (list/table/spinner/textinput/viewport components) and
  [lipgloss](https://github.com/charmbracelet/lipgloss) (styling).

Also used: [BurntSushi/toml](https://github.com/BurntSushi/toml) for
`~/.tocli/config.toml`.

## License

MIT — see [LICENSE](LICENSE).
