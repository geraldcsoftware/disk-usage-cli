//go:build darwin

// Package disk turns statfs figures into the usable free space the state
// machine reasons about.
package disk

import (
	"math"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// ReserveBytes is held back from f_bavail. APFS refuses allocating calls
// while statfs still reports roughly 32 MiB free, so anything under this
// reserve is treated as unusable.
const ReserveBytes = 64 << 20

// Usage is one reading of the container holding a path.
type Usage struct {
	Volume          string
	TotalBytes      int64
	UsableFreeBytes int64
}

// FreePct is usable free space as a percentage of the container, one decimal.
func (u Usage) FreePct() float64 {
	if u.TotalBytes == 0 {
		return 0
	}
	return math.Round(float64(u.UsableFreeBytes)/float64(u.TotalBytes)*1000) / 10
}

// FromStat converts statfs figures to a Usage.
func FromStat(fs sys.FSStat) Usage {
	free := fs.Avail*fs.BlockSize - ReserveBytes
	if free < 0 {
		free = 0
	}
	return Usage{Volume: fs.MountPoint, TotalBytes: fs.Blocks * fs.BlockSize, UsableFreeBytes: free}
}

// Read measures the container holding volumePath.
func Read(volumePath string) (Usage, error) {
	fs, err := sys.Statfs(volumePath)
	if err != nil {
		return Usage{}, err
	}
	return FromStat(fs), nil
}
