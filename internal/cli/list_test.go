package cli

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// TestList_SelfCorrectsCrashedStatus reproduces the "kill -9 the child"
// scenario end to end: a config.json claiming a torrent is running, but
// whose recorded pid has been killed out from under it without going
// through `tocli pause`. Running `list` should detect this and persist the
// corrected status, not just print something different while leaving the
// file stale.
func TestList_SelfCorrectsCrashedStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// A real, killable process stands in for a tocli-managed download
	// process; we only care that its pid becomes provably dead.
	standIn := exec.Command("sleep", "100")
	if err := standIn.Start(); err != nil {
		t.Fatalf("start stand-in process: %v", err)
	}
	pid := standIn.Process.Pid

	id := "deadbeef01"
	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	tc := &store.TorrentConfig{ID: id, Name: "stand-in", Status: store.StatusRunning, PID: pid}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	if err := standIn.Process.Kill(); err != nil { // kill -9 equivalent
		t.Fatalf("kill stand-in process: %v", err)
	}
	_ = standIn.Wait()

	// Wait for the pid to be reliably reported as dead before asserting on
	// it -- Kill() returning doesn't guarantee the kernel has finished
	// tearing the process down yet.
	deadline := time.Now().Add(2 * time.Second)
	for process.IsAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if process.IsAlive(pid) {
		t.Fatalf("stand-in process %d still alive after kill", pid)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"list"})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	if err := root.Execute(); err != nil {
		t.Fatalf("run list: %v", err)
	}

	got, err := store.LoadTorrentConfig(id)
	if err != nil {
		t.Fatalf("reload torrent config: %v", err)
	}
	if got.Status != store.StatusCrashed {
		t.Fatalf("status = %q, want %q", got.Status, store.StatusCrashed)
	}
	if got.PID != 0 {
		t.Fatalf("pid = %d, want 0", got.PID)
	}
}

// TestList_SurfacesEphemeralPortFallback confirms a torrent running on an
// OS-assigned fallback port (store.TorrentConfig.PortEphemeral) is visibly
// flagged in `list --plain` output, not just recorded silently -- the
// whole point of persisting this rather than only logging it.
func TestList_SurfacesEphemeralPortFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	id := "ephemeral1"
	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	tc := &store.TorrentConfig{ID: id, Name: "port-exhausted", Status: store.StatusRunning, PortEphemeral: true}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	out := new(bytes.Buffer)
	root := NewRootCmd("test")
	root.SetArgs([]string{"list", "--plain"})
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("run list: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "ephemeral1") || !strings.Contains(got, "ephemeral port") {
		t.Fatalf("list output missing an ephemeral-port note for a torrent with PortEphemeral=true:\n%s", got)
	}
}

// TestPause_AlreadyCrashedReportsCleanly ensures pausing a torrent whose
// process has already died reports a sensible status instead of trying (and
// failing) to signal a dead pid.
func TestPause_AlreadyCrashedReportsCleanly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const implausiblePID = 2147483647
	id := "crashed01"
	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	tc := &store.TorrentConfig{ID: id, Name: "gone", Status: store.StatusRunning, PID: implausiblePID}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	out := new(bytes.Buffer)
	root := NewRootCmd("test")
	root.SetArgs([]string{"pause", id})
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("run pause: %v", err)
	}

	if got := out.String(); !bytes.Contains([]byte(got), []byte("crashed")) {
		t.Fatalf("pause output = %q, want it to mention status crashed", got)
	}

	reloaded, err := store.LoadTorrentConfig(id)
	if err != nil {
		t.Fatalf("reload torrent config: %v", err)
	}
	if reloaded.Status != store.StatusCrashed {
		t.Fatalf("persisted status = %q, want %q", reloaded.Status, store.StatusCrashed)
	}
}
