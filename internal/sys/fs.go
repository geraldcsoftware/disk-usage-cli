//go:build darwin

package sys

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// FSStat is the subset of statfs(2) dusk uses. On APFS every volume in a
// container reports the container's totals.
type FSStat struct {
	MountPoint string
	BlockSize  int64
	Blocks     int64
	Avail      int64
}

// Statfs reads the filesystem statistics for the volume holding path.
func Statfs(path string) (FSStat, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return FSStat{}, &os.PathError{Op: "statfs", Path: path, Err: err}
	}
	return FSStat{
		MountPoint: unix.ByteSliceToString(st.Mntonname[:]),
		BlockSize:  int64(st.Bsize),
		Blocks:     int64(st.Blocks),
		Avail:      int64(st.Bavail),
	}, nil
}

// Preallocate reserves size bytes of real blocks beyond the current end of
// file without changing the logical size. A file prepared this way can be
// rewritten in place when the volume is at its allocation floor.
func Preallocate(f *os.File, size int64) error {
	fst := unix.Fstore_t{Flags: unix.F_ALLOCATEALL, Posmode: unix.F_PEOFPOSMODE, Offset: 0, Length: size}
	if err := unix.FcntlFstore(f.Fd(), unix.F_PREALLOCATE, &fst); err != nil {
		return &os.PathError{Op: "preallocate", Path: f.Name(), Err: err}
	}
	return nil
}

// AllocatedBytes is the space the inode occupies on disk, which differs from
// st_size for sparse, compressed and dataless files.
func AllocatedBytes(st *unix.Stat_t) int64 { return st.Blocks * 512 }

// ModTime returns the inode's mtime. atime is lazily maintained on the Data
// volume and is not a recency signal.
func ModTime(st *unix.Stat_t) time.Time {
	return time.Unix(st.Mtim.Sec, st.Mtim.Nsec)
}
