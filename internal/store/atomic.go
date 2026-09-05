package store

import (
	"encoding/json"
	"fmt"
	"os"
)

// writeJSONAtomic marshals v and writes it to path via write-temp-then-rename,
// so a reader (e.g. `tocli list` running concurrently with the download
// process's periodic state.json writes) never observes a half-written file.
// Rename is atomic on the same filesystem, which the temp file is guaranteed
// to be on since it's created alongside the destination.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file into place for %s: %w", path, err)
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
