//go:build darwin

package scan

import (
	"path/filepath"
	"strings"
)

// privacyProtectedRelative lists locations under the home directory where a
// process without Full Disk Access receives EPERM. Apple publishes no
// authoritative list. Sources: the Transparency, Consent and Control
// categories in System Settings (Files and Folders, Full Disk Access), Apple
// Platform Security Guide section "Protecting app access to user data", and
// EPERM observed on this list from a LaunchAgent on macOS 26 during research
// on 04/09/2026. File Provider roots (~/Library/CloudStorage children and
// ~/Library/Mobile Documents) are included because reads there can trigger
// materialisation and deletions propagate to the cloud.
var privacyProtectedRelative = []string{
	"Library/Mail",
	"Library/Messages",
	"Library/Safari",
	"Library/Cookies",
	"Library/HomeKit",
	"Library/Suggestions",
	"Library/IdentityServices",
	"Library/Metadata/CoreSpotlight",
	"Library/Application Support/AddressBook",
	"Library/Application Support/CallHistoryDB",
	"Library/Calendars",
	"Library/Containers",
	"Library/Group Containers",
	"Library/Daemon Containers",
	"Library/Mobile Documents",
	"Library/CloudStorage",
	"Desktop",
	"Documents",
	"Downloads",
}

// PrivacyProtectedRoots returns the absolute privacy protected locations for
// a home directory.
func PrivacyProtectedRoots(home string) []string {
	out := make([]string, len(privacyProtectedRelative))
	for i, rel := range privacyProtectedRelative {
		out[i] = filepath.Join(home, rel)
	}
	return out
}

// UnderPrivacyProtectedRoot reports which protected root, if any, contains
// path. Matching respects path boundaries, so ~/Library/Mailbox does not match
// ~/Library/Mail.
func UnderPrivacyProtectedRoot(path, home string) (string, bool) {
	clean := filepath.Clean(path)
	for _, root := range PrivacyProtectedRoots(home) {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return root, true
		}
	}
	return "", false
}
