//go:build darwin

package sys

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	libOnce     sync.Once
	libErr      error
	setiopolicy func(iotype, scope, policy int32) int32
	getiopolicy func(iotype, scope int32) int32
)

// loadLibSystem resolves setiopolicy_np and getiopolicy_np once. libSystem is
// always present, so a failure here means purego could not open it.
func loadLibSystem() error {
	libOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			libErr = fmt.Errorf("dlopen libSystem: %w", err)
			return
		}
		purego.RegisterLibFunc(&setiopolicy, lib, "setiopolicy_np")
		purego.RegisterLibFunc(&getiopolicy, lib, "getiopolicy_np")
	})
	return libErr
}

// DisableDatalessMaterialisation turns off automatic download of iCloud and
// File Provider placeholders for this process, then reads the policy back.
// With the policy off, reading a placeholder fails with EDEADLK instead of
// fetching it; metadata calls are unaffected.
func DisableDatalessMaterialisation() error {
	if err := loadLibSystem(); err != nil {
		return err
	}
	if rc := setiopolicy(iopolTypeVFSMaterializeDatalessFiles, iopolScopeProcess, iopolMaterializeDatalessFilesOff); rc != 0 {
		return fmt.Errorf("setiopolicy_np returned %d", rc)
	}
	off, err := DatalessMaterialisationDisabled()
	if err != nil {
		return err
	}
	if !off {
		return errors.New("materialisation policy did not read back off")
	}
	return nil
}

// DatalessMaterialisationDisabled reports whether the process policy is off.
func DatalessMaterialisationDisabled() (bool, error) {
	if err := loadLibSystem(); err != nil {
		return false, err
	}
	return getiopolicy(iopolTypeVFSMaterializeDatalessFiles, iopolScopeProcess) == iopolMaterializeDatalessFilesOff, nil
}
