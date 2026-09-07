//go:build darwin

package sys

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMaterialisationPolicyReadsBackOff(t *testing.T) {
	if err := DisableDatalessMaterialisation(); err != nil {
		t.Fatal(err)
	}
	off, err := DatalessMaterialisationDisabled()
	if err != nil || !off {
		t.Fatalf("disabled = %v, %v; want true", off, err)
	}
}

func TestStatfsIsContainerWide(t *testing.T) {
	root, err := Statfs("/")
	if err != nil {
		t.Fatal(err)
	}
	home, err := Statfs(os.Getenv("HOME"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Blocks != home.Blocks || root.BlockSize != home.BlockSize {
		t.Errorf("/ = %+v, home = %+v; APFS volumes in one container share totals", root, home)
	}
	if home.MountPoint == "" || root.MountPoint != "/" {
		t.Errorf("mount points: root %q home %q", root.MountPoint, home.MountPoint)
	}
}

func TestPreallocateAllocatesBlocksBeforeWrite(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "ballast"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	const size = 1 << 20
	if err := Preallocate(f, size); err != nil {
		t.Fatal(err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	if AllocatedBytes(&st) < size {
		t.Errorf("allocated %d, want at least %d", AllocatedBytes(&st), size)
	}
	if st.Size != 0 {
		t.Errorf("st_size %d, want 0: allocation must not change the logical size", st.Size)
	}
	if _, err := f.WriteAt([]byte("x"), 0); err != nil {
		t.Errorf("in place write after preallocation: %v", err)
	}
}

func TestFlagHelpers(t *testing.T) {
	if !HasProtectionFlag(SFRestricted) || !HasProtectionFlag(UFImmutable) || HasProtectionFlag(0) || HasProtectionFlag(SFDataless) {
		t.Error("HasProtectionFlag classifies the wrong bits")
	}
	if !IsDataless(SFDataless|0x60) || IsDataless(0x60) {
		t.Error("IsDataless classifies the wrong bits")
	}
}

func TestModTimeUsesMtime(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var st unix.Stat_t
	if err := unix.Lstat(p, &st); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(p)
	if !ModTime(&st).Equal(fi.ModTime()) {
		t.Errorf("ModTime = %v, os says %v", ModTime(&st), fi.ModTime())
	}
}
