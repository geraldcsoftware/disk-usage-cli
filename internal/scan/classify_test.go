//go:build darwin

package scan

import (
	"errors"
	"io/fs"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/geraldcsoftware/disk-usage-cli/internal/sys"
)

func pathErr(errno unix.Errno, path string) error {
	return &fs.PathError{Op: "open", Path: path, Err: errno}
}

func TestClassify(t *testing.T) {
	const home = "/Users/test"
	cases := []struct {
		name  string
		err   error
		flags uint32
		path  string
		want  Class
	}{
		{"eperm with restricted flag", pathErr(unix.EPERM, home+"/Library/Caches/x"), sys.SFRestricted, home + "/Library/Caches/x", ClassFlagProtected},
		{"eperm with datavault flag", pathErr(unix.EPERM, home+"/x"), sys.UFDatavault, home + "/x", ClassFlagProtected},
		{"eperm under mail", pathErr(unix.EPERM, home+"/Library/Mail/V10"), 0, home + "/Library/Mail/V10", ClassPrivacyProtected},
		{"eperm under containers", pathErr(unix.EPERM, home+"/Library/Containers/com.apple.Safari/Data"), 0, home + "/Library/Containers/com.apple.Safari/Data", ClassPrivacyProtected},
		{"eperm under desktop", pathErr(unix.EPERM, home+"/Desktop/a"), 0, home + "/Desktop/a", ClassPrivacyProtected},
		{"eperm elsewhere", pathErr(unix.EPERM, home+"/Library/Caches/y"), 0, home + "/Library/Caches/y", ClassSystemProtected},
		{"eacces", pathErr(unix.EACCES, home+"/x"), 0, home + "/x", ClassPermission},
		{"edeadlk", pathErr(unix.EDEADLK, home+"/Library/Mobile Documents/a"), 0, home + "/Library/Mobile Documents/a", ClassCloudOnly},
		{"enoent", pathErr(unix.ENOENT, home+"/x"), 0, home + "/x", ClassVanished},
		{"unrelated", errors.New("boom"), 0, home + "/x", ClassOther},
	}
	for _, c := range cases {
		if got := Classify(c.err, c.flags, c.path, home); got != c.want {
			t.Errorf("%s: Classify = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUnderPrivacyProtectedRoot(t *testing.T) {
	const home = "/Users/test"
	root, ok := UnderPrivacyProtectedRoot(home+"/Library/Metadata/CoreSpotlight/index", home)
	if !ok || root != home+"/Library/Metadata/CoreSpotlight" {
		t.Errorf("CoreSpotlight: %q %v", root, ok)
	}
	if _, ok := UnderPrivacyProtectedRoot(home+"/Library/Caches/foo", home); ok {
		t.Error("Caches is not privacy protected")
	}
	if _, ok := UnderPrivacyProtectedRoot(home+"/Library/Mailbox", home); ok {
		t.Error("prefix match must respect path boundaries")
	}
}

func TestIsPackageLeaf(t *testing.T) {
	for _, yes := range []string{"Safari.app", "Photos Library.photoslibrary", "Build.xcarchive", "Foo.framework", "Bar.bundle", "Clip.fcpbundle"} {
		if !IsPackageLeaf(yes) {
			t.Errorf("%q should be a package leaf", yes)
		}
	}
	for _, no := range []string{"cache", "app", "x.appx", "notes.txt"} {
		if IsPackageLeaf(no) {
			t.Errorf("%q should not be a package leaf", no)
		}
	}
}
