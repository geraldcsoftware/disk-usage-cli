//go:build darwin

package scan

import (
	"errors"

	"golang.org/x/sys/unix"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

// Class names why an entry could not be measured. None of these abort a scan.
type Class string

const (
	ClassFlagProtected    Class = "flag_protected"    // EPERM with an immutable, restricted or data vault flag
	ClassPrivacyProtected Class = "privacy_protected" // EPERM under a known privacy protected location
	ClassSystemProtected  Class = "system_protected"  // EPERM anywhere else
	ClassPermission       Class = "permission"        // EACCES: ownership or mode
	ClassCloudOnly        Class = "cloud_only"        // EDEADLK: dataless content with materialisation off
	ClassVanished         Class = "vanished"          // ENOENT during the walk
	ClassOther            Class = "other"
)

// Classify maps a filesystem error to a Class. flags are the st_flags of the
// entry or its parent when known, 0 otherwise.
func Classify(err error, flags uint32, path, home string) Class {
	var errno unix.Errno
	if !errors.As(err, &errno) {
		return ClassOther
	}
	switch errno {
	case unix.EPERM:
		if sys.HasProtectionFlag(flags) {
			return ClassFlagProtected
		}
		if _, ok := UnderPrivacyProtectedRoot(path, home); ok {
			return ClassPrivacyProtected
		}
		return ClassSystemProtected
	case unix.EACCES:
		return ClassPermission
	case unix.EDEADLK:
		return ClassCloudOnly
	case unix.ENOENT:
		return ClassVanished
	}
	return ClassOther
}
