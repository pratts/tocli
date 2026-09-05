// Package humanize renders byte counts and rates for display.
package humanize

import "fmt"

// Bytes renders n as a human-readable size, e.g. "1.9 MiB".
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Rate renders a bytes-per-second rate, e.g. "512.0 KiB/s".
func Rate(bytesPerSec float64) string {
	return Bytes(int64(bytesPerSec)) + "/s"
}
