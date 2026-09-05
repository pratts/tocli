package store

import "testing"

func TestValidateID_AcceptsWellFormedIDs(t *testing.T) {
	for _, id := range []string{"a1b2c3d4e5", "deadbeef01", "ABCXYZ0123", "a"} {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}
}

func TestValidateID_RejectsPathTraversalAttempts(t *testing.T) {
	for _, id := range []string{
		"..",
		"../..",
		"../../etc/passwd",
		"a/../../b",
		"/etc/passwd",
		"a/b",
		`a\b`,
		"",
		".",
		"a.b",
		"a b",
		"a\x00b",
	} {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) = nil, want an error", id)
		}
	}
}

func TestTorrentDir_RejectsInvalidID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := TorrentDir("../../../etc"); err == nil {
		t.Fatal("expected TorrentDir to reject a path-traversal id")
	}

	// Confirm every other per-torrent path helper inherits the same
	// protection, since they're all built on TorrentDir.
	for name, fn := range map[string]func(string) (string, error){
		"ConfigPath":   ConfigPath,
		"StatePath":    StatePath,
		"MetainfoPath": MetainfoPath,
		"LockPath":     LockPath,
		"LogPath":      LogPath,
	} {
		if _, err := fn("../../../etc"); err == nil {
			t.Errorf("%s did not reject a path-traversal id", name)
		}
	}

	if err := InitTorrentDir("../../../etc"); err == nil {
		t.Fatal("InitTorrentDir did not reject a path-traversal id")
	}
}
