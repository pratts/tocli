//go:build !linux && !darwin

package process

import "errors"

// currentBootID has no known implementation outside linux/darwin. Callers
// (see store.ReconcileLiveness) treat an error here as "can't tell, fall
// back to the plain pid check" rather than failing outright.
func currentBootID() (string, error) {
	return "", errors.New("boot id detection not implemented on this platform")
}
