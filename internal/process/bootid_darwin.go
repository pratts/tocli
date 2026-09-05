//go:build darwin

package process

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// currentBootID has no direct equivalent of Linux's boot_id on macOS, so we
// use the kernel's recorded boot time (kern.boottime) as a surrogate: it's
// set once at boot and reappearing at the exact same microsecond across two
// separate boots is implausible enough for our purposes (detecting pid
// reuse, not cryptographic uniqueness).
func currentBootID() (string, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return "", fmt.Errorf("read boot time: %w", err)
	}
	return fmt.Sprintf("%d.%06d", tv.Sec, tv.Usec), nil
}
