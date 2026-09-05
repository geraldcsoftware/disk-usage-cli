//go:build darwin

package scan

import (
	"path/filepath"
	"strings"
)

// packageExtensions name directories Finder shows as single objects. They are
// measured as one unit and never entered for deletion.
var packageExtensions = map[string]bool{
	".app": true, ".photoslibrary": true, ".xcarchive": true,
	".framework": true, ".bundle": true, ".fcpbundle": true,
}

// IsPackageLeaf reports whether a directory name denotes a package.
func IsPackageLeaf(name string) bool {
	return packageExtensions[strings.ToLower(filepath.Ext(name))]
}
