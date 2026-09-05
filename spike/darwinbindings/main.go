// PROTOTYPE. Throwaway spike for wayfinder ticket #6: do the four darwin calls
// the dusk design depends on work from a Go program built with CGO_ENABLED=0?
//
// Run from this directory:
//
//	CGO_ENABLED=0 go run .
//
// Nothing here is production code. It prints the observed state after each
// probe so the reader can judge the answer directly.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Constants absent from golang.org/x/sys/unix, with their SDK header sources.
const (
	// usr/include/sys/fcntl.h lines 184 and 185 (macOS 26.4 SDK).
	atSymlinkNofollowAny = 0x0800
	atResolveBeneath     = 0x2000
	// usr/include/sys/resource.h lines 509, 520, 539 to 541.
	iopolTypeVFSMaterializeDatalessFiles = 3
	iopolScopeProcess                    = 0
	iopolMaterializeDatalessFilesOff     = 1
	// usr/include/sys/stat.h line 359.
	sfDataless = 0x40000000
)

func main() {
	fmt.Printf("go %s %s/%s, purego %s\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, puregoVersion())
	ok1 := probeMaterialisationPolicy()
	ok2 := probeUnlinkat()
	ok3 := probePreallocate()
	ok4 := probeStatfs()
	fmt.Println("\n=== verdict ===")
	for _, r := range []struct {
		name string
		ok   bool
	}{{"setiopolicy_np via purego", ok1}, {"unlinkat with NOFOLLOW_ANY|RESOLVE_BENEATH", ok2}, {"F_PREALLOCATE and in place pwrite", ok3}, {"statfs container semantics", ok4}} {
		fmt.Printf("%-45s %s\n", r.name, pass(r.ok))
	}
}

func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func puregoVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/ebitengine/purego" {
				return d.Version
			}
		}
	}
	return "unknown"
}

// Probe 1: set the dataless materialisation policy off through libSystem and
// read it back. Only when the read back says off is a dataless file opened
// and read; either call must fail with EDEADLK, no byte may arrive, and
// st_blocks and SF_DATALESS must be unchanged afterwards.
func probeMaterialisationPolicy() bool {
	fmt.Println("=== probe 1: setiopolicy_np ===")
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		fmt.Println("dlopen libSystem:", err)
		return false
	}
	var setiopolicy func(iotype, scope, policy int32) int32
	var getiopolicy func(iotype, scope int32) int32
	purego.RegisterLibFunc(&setiopolicy, lib, "setiopolicy_np")
	purego.RegisterLibFunc(&getiopolicy, lib, "getiopolicy_np")

	before := getiopolicy(iopolTypeVFSMaterializeDatalessFiles, iopolScopeProcess)
	rc := setiopolicy(iopolTypeVFSMaterializeDatalessFiles, iopolScopeProcess, iopolMaterializeDatalessFilesOff)
	after := getiopolicy(iopolTypeVFSMaterializeDatalessFiles, iopolScopeProcess)
	fmt.Printf("policy before=%d  setiopolicy_np rc=%d  policy after=%d (want %d)\n", before, rc, after, iopolMaterializeDatalessFilesOff)
	if rc != 0 || after != iopolMaterializeDatalessFilesOff {
		fmt.Println("policy did not read back off; skipping the dataless open so nothing is downloaded")
		return false
	}

	path, st, found := findDatalessFile(filepath.Join(os.Getenv("HOME"), "Library", "Mobile Documents"))
	if !found {
		fmt.Println("no non empty dataless fixture found under Mobile Documents; policy read back off, open test skipped")
		return true
	}
	fmt.Printf("fixture: %s/(name withheld)\n  flags=%#x blocks=%d size=%d\n", shorten(filepath.Dir(path)), st.Flags, st.Blocks, st.Size)
	fd, openErr := unix.Open(path, unix.O_RDONLY, 0)
	var readErr error
	n := 0
	if openErr == nil {
		n, readErr = unix.Read(fd, make([]byte, 1))
		unix.Close(fd)
	}
	var st2 unix.Stat_t
	_ = unix.Lstat(path, &st2)
	fmt.Printf("open(O_RDONLY) err=%v\nread(1 byte)   n=%d err=%v (want EDEADLK on open or read)\n  blocks after=%d flags after=%#x (want unchanged, still dataless)\n", openErr, n, readErr, st2.Blocks, st2.Flags)
	refused := errors.Is(openErr, unix.EDEADLK) || errors.Is(readErr, unix.EDEADLK)
	// unix.Read reports -1 on error, so "no byte arrived" is n <= 0.
	return refused && n <= 0 && st2.Blocks == st.Blocks && st2.Flags&sfDataless != 0
}

func findDatalessFile(root string) (string, unix.Stat_t, bool) {
	var found string
	var st unix.Stat_t
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.Type().IsRegular() {
			var s unix.Stat_t
			// Size must be non zero: a dataless placeholder with no data
			// has nothing to materialise and opens without EDEADLK.
			if unix.Lstat(p, &s) == nil && s.Flags&sfDataless != 0 && s.Size > 0 {
				found, st = p, s
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, st, found != ""
}

// Probe 2: build a tree, open its root with O_DIRECTORY|O_NOFOLLOW, then
// unlinkat with both flags: a plain file must go, a symlink component inside
// the root must be rejected, and a .. escape must be rejected.
func probeUnlinkat() bool {
	fmt.Println("\n=== probe 2: unlinkat flags ===")
	base, err := os.MkdirTemp("", "dusk-spike-")
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer os.RemoveAll(base)
	root := filepath.Join(base, "root")
	must(os.MkdirAll(filepath.Join(root, "real"), 0o755))
	must(os.WriteFile(filepath.Join(root, "real", "victim.txt"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(root, "real", "via-link.txt"), []byte("x"), 0o644))
	must(os.Symlink("real", filepath.Join(root, "link")))
	must(os.WriteFile(filepath.Join(base, "outside.txt"), []byte("x"), 0o644))

	fd, err := unix.Open(root, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY, 0)
	if err != nil {
		fmt.Println("open root:", err)
		return false
	}
	defer unix.Close(fd)
	flags := atSymlinkNofollowAny | atResolveBeneath

	var st unix.Stat_t
	err = unix.Fstatat(fd, "real/victim.txt", &st, flags)
	fmt.Printf("fstatat(real/victim.txt, flags) err=%v ino=%d\n", err, st.Ino)
	statOK := err == nil

	err = unix.Unlinkat(fd, "real/victim.txt", flags)
	_, statErr := os.Lstat(filepath.Join(root, "real", "victim.txt"))
	fmt.Printf("unlinkat(real/victim.txt)      err=%v  file gone=%v (want gone)\n", err, errors.Is(statErr, os.ErrNotExist))
	plainOK := err == nil && errors.Is(statErr, os.ErrNotExist)

	err = unix.Unlinkat(fd, "link/via-link.txt", flags)
	_, statErr = os.Lstat(filepath.Join(root, "real", "via-link.txt"))
	fmt.Printf("unlinkat(link/via-link.txt)    err=%v  target intact=%v (want rejected, intact)\n", err, statErr == nil)
	linkOK := err != nil && statErr == nil

	err = unix.Unlinkat(fd, "../outside.txt", flags)
	_, statErr = os.Lstat(filepath.Join(base, "outside.txt"))
	fmt.Printf("unlinkat(../outside.txt)       err=%v  target intact=%v (want rejected, intact)\n", err, statErr == nil)
	escapeOK := err != nil && statErr == nil

	err = unix.Unlinkat(fd, "link/via-link.txt", 0)
	fmt.Printf("control: unlinkat(link/via-link.txt) with flags=0 err=%v (expected to succeed, showing the flags do the work)\n", err)

	return statOK && plainOK && linkOK && escapeOK
}

// Probe 3: F_PREALLOCATE with F_ALLOCATEALL must allocate blocks immediately,
// and the file must then accept an in place pwrite at offset 0.
func probePreallocate() bool {
	fmt.Println("\n=== probe 3: F_PREALLOCATE ===")
	f, err := os.CreateTemp("", "dusk-spike-ballast-")
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer os.Remove(f.Name())
	defer f.Close()
	const want = 8 << 20
	fst := unix.Fstore_t{Flags: unix.F_ALLOCATEALL, Posmode: unix.F_PEOFPOSMODE, Offset: 0, Length: want}
	err = unix.FcntlFstore(f.Fd(), unix.F_PREALLOCATE, &fst)
	var st unix.Stat_t
	_ = unix.Fstat(int(f.Fd()), &st)
	fmt.Printf("fcntl(F_PREALLOCATE, F_ALLOCATEALL, %d) err=%v bytesalloc=%d\n  st_size=%d st_blocks*512=%d (want >= %d)\n", want, err, fst.Bytesalloc, st.Size, st.Blocks*512, want)
	allocOK := err == nil && st.Blocks*512 >= want

	n, err := unix.Pwrite(int(f.Fd()), []byte(`{"schema":1}`), 0)
	_ = unix.Fstat(int(f.Fd()), &st)
	fmt.Printf("pwrite(offset 0) n=%d err=%v  st_size=%d st_blocks*512=%d\n", n, err, st.Size, st.Blocks*512)
	return allocOK && err == nil && n == 12
}

// Probe 4: statfs on the home directory and on / must report the same
// container wide free space.
func probeStatfs() bool {
	fmt.Println("\n=== probe 4: statfs ===")
	var home, rootfs unix.Statfs_t
	if err := unix.Statfs(os.Getenv("HOME"), &home); err != nil {
		fmt.Println(err)
		return false
	}
	if err := unix.Statfs("/", &rootfs); err != nil {
		fmt.Println(err)
		return false
	}
	pr := func(name string, s unix.Statfs_t) {
		fmt.Printf("%-6s mnt=%s bsize=%d blocks=%d bavail=%d  total=%.1f GB usable=%.1f GB\n", name, unix.ByteSliceToString(s.Mntonname[:]), s.Bsize, s.Blocks, s.Bavail,
			float64(s.Blocks)*float64(s.Bsize)/1e9, float64(s.Bavail)*float64(s.Bsize)/1e9)
	}
	pr("~", home)
	pr("/", rootfs)
	same := home.Blocks == rootfs.Blocks && home.Bavail == rootfs.Bavail && home.Bsize == rootfs.Bsize
	fmt.Printf("identical blocks and bavail across volumes: %v (want true)\n", same)
	return same
}

func shorten(p string) string {
	if h := os.Getenv("HOME"); h != "" && len(p) > len(h) && p[:len(h)] == h {
		return "~" + p[len(h):]
	}
	return p
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
