package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrDatabaseLocked = errors.New("coco: database is locked by another process")

// FileLock manages process-level database exclusion using a lock file.
//
// The exclusion is an operating-system lock held on the open lock file (flock on Unix,
// LockFileEx on Windows), not the file's existence. The operating system drops the lock
// when the process exits for any reason, so a crash, a kill or a power loss never leaves
// the database locked. A leftover lock file is harmless and is reused.
type FileLock struct {
	mu       sync.Mutex
	lockPath string
	file     *os.File
	locked   bool
}

// AcquireLock attempts to acquire an exclusive lock for the database path. It fails with
// ErrDatabaseLocked when another process, or another open handle in this process, holds it.
func AcquireLock(dbPath string) (*FileLock, error) {
	if dbPath == ":memory:" || dbPath == "" {
		return &FileLock{locked: true}, nil
	}
	lockPath := dbPath + "-lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errWouldBlock) {
			return nil, fmt.Errorf("%w: %s", ErrDatabaseLocked, lockPath)
		}
		return nil, fmt.Errorf("lock %s: %w", lockPath, err)
	}

	// The owner's pid, for people looking at the file. It plays no part in locking.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0)
		_ = f.Sync()
	}

	return &FileLock{
		lockPath: lockPath,
		file:     f,
		locked:   true,
	}, nil
}

// Release releases the lock. The lock file is left in place: removing it would let a
// process that opened the old file and a process that creates a new one both believe
// they hold the lock.
func (l *FileLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.locked || l.file == nil {
		return nil
	}
	l.locked = false
	err := unlockFile(l.file)
	if cerr := l.file.Close(); err == nil {
		err = cerr
	}
	l.file = nil
	return err
}
