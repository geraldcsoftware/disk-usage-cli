//go:build darwin

// Package sys holds the darwin calls and constants dusk needs that
// golang.org/x/sys/unix does not provide.
package sys

// Constants absent from golang.org/x/sys/unix v0.47.0. Values and sources are
// from the macOS 26.4 SDK under $(xcrun --show-sdk-path)/usr/include.
const (
	// sys/fcntl.h line 184: the path may contain no symlink at all.
	AtSymlinkNofollowAny = 0x0800
	// sys/fcntl.h line 185: the path must stay beneath the starting directory.
	AtResolveBeneath = 0x2000

	// sys/stat.h st_flags bits. Lines 344 to 359.
	UFImmutable  = 0x00000002
	UFAppend     = 0x00000004
	UFDatavault  = 0x00000080
	SFImmutable  = 0x00020000
	SFAppend     = 0x00040000
	SFRestricted = 0x00080000
	SFDataless   = 0x40000000

	// sys/resource.h lines 509, 520 and 540.
	iopolTypeVFSMaterializeDatalessFiles = 3
	iopolScopeProcess                    = 0
	iopolMaterializeDatalessFilesOff     = 1
)

// HasProtectionFlag reports whether st_flags carries a bit that makes the
// kernel answer EPERM regardless of ownership.
func HasProtectionFlag(flags uint32) bool {
	return flags&(UFImmutable|SFImmutable|UFAppend|SFAppend|SFRestricted|UFDatavault) != 0
}

// IsDataless reports whether the inode is a cloud placeholder whose content is
// not on disk.
func IsDataless(flags uint32) bool { return flags&SFDataless != 0 }
