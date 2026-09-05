// Package addflow implements the interactive `tocli start` screen: an
// optional source-input prompt, a loading spinner while metadata resolves,
// and a preview (file tree, trackers, live peer count) with confirm/cancel.
//
// All torrent/process/store logic is delegated to internal/engine:
// OpenPreview creates and holds the live torrent.Client used for both
// resolution and the live stats shown here, and StartTorrent (called on
// confirm) is the exact same function the plain, non-interactive `tocli
// start` command calls. This package only renders state and translates
// keypresses into calls against that state -- see model.go's Update for
// the full state machine.
package addflow

import (
	"context"
	"errors"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/store"
)

type stage int

const (
	stageInput stage = iota
	stageLoading
	stagePreview
	stageStarting
	stageError
)

// statsTickInterval matches the child's own state.json write interval, so
// the live peer count shown during preview refreshes at the same cadence
// list/dashboard use once the torrent is actually running.
const statsTickInterval = 2 * time.Second

// Model is the add-flow's single top-level tea.Model. Stage-specific
// update/view logic lives in input.go, loading.go, and preview.go; this
// file holds shared state and the top-level dispatch.
type Model struct {
	stage     stage
	globalCfg config.Config

	// Set once construction knows the source (either passed as an
	// argument, or typed into the input stage).
	source string

	textInput textinput.Model
	spinner   spinner.Model

	session *engine.PreviewSession // live only during stagePreview/stageStarting
	err     error

	// cancelPreview aborts an in-flight engine.OpenPreview call. Set only
	// while stage == stageLoading; calling it lets Esc abandon a slow
	// resolution immediately instead of leaking the torrent.Client for the
	// rest of engine.MetadataTimeout.
	cancelPreview context.CancelFunc

	// Preview stage: viewport holds the pre-rendered file tree (static once
	// the preview is entered); name/totalSize/trackers are likewise fixed
	// at that point. Live peer/seeder stats are read fresh from m.session
	// on every render instead, via statsTickMsg.
	viewport  viewport.Model
	name      string
	totalSize int64
	trackers  []string

	width, height int

	// Result, read by the caller once tui.Run returns.
	Cancelled   bool
	Started     bool
	StartedID   string
	StartedName string
}

// New builds the add-flow model. If source is empty, the flow starts at
// the input screen; otherwise it goes straight to resolving it.
func New(source string, globalCfg config.Config) Model {
	m := Model{globalCfg: globalCfg}
	if source == "" {
		m.stage = stageInput
		m.textInput = newTextInput()
	} else {
		m.stage = stageLoading
		m.source = source
		m.spinner = newSpinner()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.stage == stageLoading {
		// Init can only return a tea.Cmd, not an updated model, so it can't
		// itself store the cancel func startLoading creates. Route through
		// a message instead: updateLoading's beginLoadingMsg case runs
		// startLoading from inside Update, where the returned model (with
		// cancelPreview set) actually persists.
		return func() tea.Msg { return beginLoadingMsg{} }
	}
	return textinput.Blink
}

// startLoading transitions into the loading stage and kicks off metadata
// resolution for source, bounded by engine.MetadataTimeout and cancellable
// early via cancelPreview (see the Esc handling in updateLoading).
func (m Model) startLoading(source string) (Model, tea.Cmd) {
	m.source = source
	m.stage = stageLoading
	m.spinner = newSpinner()
	ctx, cancel := context.WithTimeout(context.Background(), engine.MetadataTimeout)
	m.cancelPreview = cancel
	return m, tea.Batch(m.spinner.Tick, openPreviewCmd(ctx, source))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = ws.Width, ws.Height
	}

	// Global quit: cancel cleanly from any stage.
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		m.closeSession()
		m.Cancelled = true
		return m, tea.Quit
	}

	switch m.stage {
	case stageInput:
		return m.updateInput(msg)
	case stageLoading:
		return m.updateLoading(msg)
	case stagePreview:
		return m.updatePreview(msg)
	case stageStarting:
		return m.updateStarting(msg)
	case stageError:
		return m.updateError(msg)
	}
	return m, nil
}

func (m Model) View() string {
	switch m.stage {
	case stageInput:
		return m.viewInput()
	case stageLoading:
		return m.viewLoading()
	case stagePreview:
		return m.viewPreview()
	case stageStarting:
		return m.viewStarting()
	case stageError:
		return m.viewError()
	}
	return ""
}

// closeSession cancels any in-flight metadata resolution and releases the
// preview session, if either is active. Safe to call more than once or
// when neither is active.
func (m *Model) closeSession() {
	m.stopWaitingForMetadata()
	if m.session != nil {
		_ = m.session.Close()
		m.session = nil
	}
}

// stopWaitingForMetadata cancels an in-flight engine.OpenPreview call, if
// one is active. Called both when abandoning it early (Esc during loading)
// and when it's already concluded (success or failure), so its underlying
// timer is released promptly rather than sitting armed until
// engine.MetadataTimeout naturally elapses.
func (m *Model) stopWaitingForMetadata() {
	if m.cancelPreview != nil {
		m.cancelPreview()
		m.cancelPreview = nil
	}
}

func newTextInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "path/to/file.torrent or magnet:?xt=..."
	ti.Focus()
	ti.CharLimit = 2048
	ti.Width = 60
	return ti
}

func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return s
}

// beginLoadingMsg kicks off metadata resolution from Init (see Init's
// comment for why it can't just call startLoading directly).
type beginLoadingMsg struct{}

// previewReadyMsg/previewErrMsg are delivered once openPreviewCmd's
// goroutine finishes.
type previewReadyMsg struct{ session *engine.PreviewSession }
type previewErrMsg struct{ err error }
type statsTickMsg struct{}
type startedMsg struct{ config *store.TorrentConfig }
type startErrMsg struct {
	err      error
	existing *store.TorrentConfig
}

// openPreviewCmd resolves source's metadata via engine.OpenPreview,
// running on its own goroutine (per bubbletea convention) so the spinner
// keeps animating instead of the UI blocking on network I/O. ctx bounds
// (and can cancel) the wait; see startLoading.
func openPreviewCmd(ctx context.Context, source string) tea.Cmd {
	return func() tea.Msg {
		session, err := engine.OpenPreview(ctx, source)
		if err != nil {
			return previewErrMsg{err}
		}
		return previewReadyMsg{session}
	}
}

func tickStatsCmd() tea.Cmd {
	return tea.Tick(statsTickInterval, func(time.Time) tea.Msg { return statsTickMsg{} })
}

// startTorrentCmd finalizes the previewed torrent: it hands the resolved
// metainfo to engine.StartTorrent -- the same function the plain CLI start
// command calls -- which caches the metainfo, writes config.json, and
// spawns the detached background process that performs the actual
// download. The preview session is closed first: it only ever resolved
// metadata and watched peer/seeder stats (DownloadAll was never called on
// it, since it's rooted at a scratch directory, not the torrent's real
// save path), so there's nothing to lose by releasing it before the real
// download's own client starts up.
func startTorrentCmd(session *engine.PreviewSession, source string, cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		mi := session.Metainfo()
		_ = session.Close()

		tc, err := engine.StartTorrent(mi, source, cfg)
		if err != nil {
			var existing *store.TorrentConfig
			if errors.Is(err, engine.ErrAlreadyTracked) {
				existing = tc
			}
			return startErrMsg{err: err, existing: existing}
		}
		return startedMsg{config: tc}
	}
}
