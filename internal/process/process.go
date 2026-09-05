// Package process handles the process-per-torrent lifecycle: spawning a
// detached child, probing whether a pid is still alive, and signaling it.
//
// This relies on POSIX signals (SIGTERM/SIGKILL) and process groups, so it
// only targets Unix-like systems (macOS/Linux), matching the environment
// tocli is built for.
package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// SpawnDetached starts exe with args as a new session leader (setsid), so
// the child:
//   - keeps running after this process (the CLI invocation) exits, since it
//     no longer belongs to the parent's process group;
//   - doesn't receive signals meant for the parent's controlling terminal,
//     e.g. Ctrl-C (SIGINT) or a terminal hangup (SIGHUP) when the shell that
//     ran `tocli start` closes.
//
// stdout/stderr are redirected to logPath since the child has no terminal
// to print to; stdin is /dev/null for the same reason. We deliberately
// don't call cmd.Wait(): the child is meant to outlive this process, and
// once we exit it's reparented to init/launchd, which reaps it normally.
func SpawnDetached(exe string, args []string, logPath string) (int, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file %s: %w", logPath, err)
	}
	defer logFile.Close() // safe: the child received its own dup'd fd at fork time

	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start background process: %w", err)
	}
	return cmd.Process.Pid, nil
}

// IsAlive reports whether pid refers to a live process. Signal 0 performs
// the kernel's existence/permission checks without actually delivering a
// signal, which is the standard way to probe a pid without side effects.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but is owned by someone else; still
	// alive as far as we're concerned. Anything else (notably ESRCH) means
	// no such process.
	return errors.Is(err, syscall.EPERM)
}

// Signal delivers sig to pid.
func Signal(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(pid, sig); err != nil {
		return fmt.Errorf("signal pid %d with %v: %w", pid, sig, err)
	}
	return nil
}

// Terminate asks pid to shut down gracefully (SIGTERM) and waits up to
// timeout for it to exit, escalating to SIGKILL if it hasn't.
func Terminate(pid int, timeout time.Duration) error {
	if !IsAlive(pid) {
		return nil
	}
	if err := Signal(pid, syscall.SIGTERM); err != nil {
		return err
	}

	const pollInterval = 100 * time.Millisecond
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(pollInterval)
	}

	if !IsAlive(pid) {
		return nil
	}
	if err := Signal(pid, syscall.SIGKILL); err != nil {
		return err
	}
	return nil
}
