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
type FileLock struct {
	mu       sync.Mutex
	lockPath string
	file     *os.File
	locked   bool
}

// AcquireLock attempts to acquire an exclusive lock for the database path.
func AcquireLock(dbPath string) (*FileLock, error) {
	if dbPath == ":memory:" || dbPath == "" {
		return &FileLock{locked: true}, nil
	}
	lockPath := dbPath + "-lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}

	// Try to open exclusively
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrDatabaseLocked, lockPath)
		}
		return nil, err
	}

	pid := os.Getpid()
	fmt.Fprintf(f, "%d\n", pid)
	_ = f.Sync()

	return &FileLock{
		lockPath: lockPath,
		file:     f,
		locked:   true,
	}, nil
}

// Release releases the lock and removes the lock file.
func (l *FileLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.locked || l.file == nil {
		return nil
	}
	l.locked = false
	_ = l.file.Close()
	l.file = nil
	if l.lockPath != "" {
		_ = os.Remove(l.lockPath)
	}
	return nil
}
