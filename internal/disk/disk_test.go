//go:build darwin

package disk

import (
	"os"
	"testing"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

func TestFromStatSubtractsReserve(t *testing.T) {
	u := FromStat(sys.FSStat{MountPoint: "/System/Volumes/Data", BlockSize: 4096, Blocks: 59_840_624, Avail: 10_069_912})
	if u.Volume != "/System/Volumes/Data" {
		t.Errorf("volume = %q", u.Volume)
	}
	if u.TotalBytes != 4096*59_840_624 {
		t.Errorf("total = %d", u.TotalBytes)
	}
	if want := int64(4096*10_069_912) - ReserveBytes; u.UsableFreeBytes != want {
		t.Errorf("usable free = %d, want %d", u.UsableFreeBytes, want)
	}
	if got := u.FreePct(); got != 16.8 {
		t.Errorf("free pct = %v, want 16.8", got)
	}
}

func TestFromStatClampsAtFloor(t *testing.T) {
	u := FromStat(sys.FSStat{BlockSize: 4096, Blocks: 1000, Avail: 10})
	if u.UsableFreeBytes != 0 {
		t.Errorf("usable free below the reserve must clamp to 0, got %d", u.UsableFreeBytes)
	}
}

func TestReadMatchesRoot(t *testing.T) {
	home, err := Read(os.Getenv("HOME"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := Read("/")
	if err != nil {
		t.Fatal(err)
	}
	if home.TotalBytes != root.TotalBytes {
		t.Errorf("home total %d, root total %d; expected one container", home.TotalBytes, root.TotalBytes)
	}
	if _, err := Read("/no/such/path"); err == nil {
		t.Error("missing path must error")
	}
}
