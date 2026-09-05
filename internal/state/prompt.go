//go:build darwin

package state

import (
	"errors"
	"os"
	"strings"
)

// WritePrompt replaces the prompt file atomically. A failure leaves the
// previous prompt in place, which is why status.json remains the source of
// truth for the recommended Starship module.
func (d Dir) WritePrompt(text string) error {
	tmp, err := os.CreateTemp(string(d), "prompt-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, d.Path("prompt")); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// ReadPrompt returns the prompt text, empty when no prompt exists.
func (d Dir) ReadPrompt() (string, error) {
	raw, err := os.ReadFile(d.Path("prompt"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(raw), "\n"), nil
}
