//go:build windows

package file

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var errWouldBlock = errors.New("lock held elsewhere")

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
	errorIOPending          = syscall.Errno(997)
)

// lockFile takes an exclusive, non-blocking LockFileEx lock on the first byte. The lock
// belongs to the file handle, so a second handle in this process is refused too, and
// Windows releases it when the handle is closed or the process ends.
func lockFile(f *os.File) error {
	var ol syscall.Overlapped
	r, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0, // reserved
		1, // lock one byte
		0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r != 0 {
		return nil
	}
	if errors.Is(err, errorLockViolation) || errors.Is(err, errorIOPending) {
		return errWouldBlock
	}
	return err
}

func unlockFile(f *os.File) error {
	var ol syscall.Overlapped
	r, _, err := procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if r != 0 {
		return nil
	}
	return err
}
