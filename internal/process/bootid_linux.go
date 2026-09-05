//go:build linux

package process

import (
	"fmt"
	"os"
	"strings"
)

// currentBootID reads the kernel's boot_id: a random UUID generated fresh
// on every boot. See boot_id(5) / random(4).
func currentBootID() (string, error) {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("read boot id: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
