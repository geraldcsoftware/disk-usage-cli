//go:build darwin

// Package state owns the files under the dusk state directory: status,
// prompt, samples and the instance lock. Writers are designed to succeed at
// the APFS allocation floor.
package state

import (
	"os"
	"path/filepath"
)

// Dir is the absolute path of the state directory.
type Dir string

// DefaultDir resolves $XDG_STATE_HOME/dusk or ~/.local/state/dusk and creates
// it with mode 0700.
func DefaultDir(getenv func(string) string, home string) (Dir, error) {
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "dusk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return Dir(dir), nil
}

// Path returns the absolute path of a file inside the directory.
func (d Dir) Path(name string) string { return filepath.Join(string(d), name) }
