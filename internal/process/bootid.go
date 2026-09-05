package process

// bootIDFunc resolves an identifier for the current boot session.
// currentBootID is platform-specific (see bootid_linux.go, bootid_darwin.go,
// bootid_other.go); it's stored in a var purely so it's replaceable, though
// callers that need to fake a reboot for testing should prefer overriding
// the seam in internal/store (see store.currentBootID), which is what
// actually needs to be test-injectable across package boundaries.
var bootIDFunc = currentBootID

// BootID returns an identifier that's stable for the life of the current
// boot session and changes across a reboot. It exists to detect pid reuse:
// a pid recorded before a reboot must never be trusted as "the same
// process" just because a live process happens to have that number now.
func BootID() (string, error) {
	return bootIDFunc()
}
