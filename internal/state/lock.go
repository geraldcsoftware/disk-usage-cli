//go:build darwin

package state

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// ErrLocked means another dusk instance holds the state directory.
var ErrLocked = errors.New("another dusk instance is running")

// Lock takes an exclusive, non blocking flock on the lock file. The returned
// function releases it.
func (d Dir) Lock() (func(), error) {
	f, err := os.OpenFile(d.Path("lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
	}, nil
}
