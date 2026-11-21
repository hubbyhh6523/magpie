package sysutil

import (
	"errors"
	"syscall"
)

// BlockedError is a new version that couldn't be started on this computer.
// Nothing of it is left in place: the version running is still the one
// installed.
type BlockedError struct {
	Err error
}

// errAccessDisabledByPolicy is AppLocker's and software restriction
// policies' refusal; ERROR_SYSTEM_INTEGRITY_* (4550…4599) are code
// integrity's, Smart App Control's among them ("An Application Control
// policy has blocked this file", ERROR_SYSTEM_INTEGRITY_POLICY_VIOLATION).
const (
	errAccessDisabledByPolicy syscall.Errno = 1260
	errIntegrityPolicy        syscall.Errno = 4551
)

func policyRefused(err error) bool {
	var n syscall.Errno
	if !errors.As(err, &n) {
		return false
	}
	return n == errAccessDisabledByPolicy || n >= 4550 && n < 4600
}

func (e *BlockedError) Error() string {
	if e.Policy() {
		return "Windows didn't let the new version start (" + e.Err.Error() + "): Smart App Control or another application control policy blocks magpie's unsigned builds; download it from " + Site + " and run it yourself, or turn Smart App Control off, then check for updates again"
	}
	return "the new version couldn't start: " + e.Err.Error() + "; download it from " + Site + " and run it yourself"
}

func (e *BlockedError) Unwrap() error { return e.Err }

func (e *BlockedError) Policy() bool { return policyRefused(e.Err) }
