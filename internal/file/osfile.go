package file

import (
	"os"
	"sync"
)

// OSBackend provides a disk-backed Backend implementation using os.File.
type OSBackend struct {
	mu   sync.RWMutex
	file *os.File
	path string
}

// OpenOSBackend opens or creates a file on disk.
func OpenOSBackend(path string, readOnly bool) (*OSBackend, error) {
	flags := os.O_RDWR | os.O_CREATE
	if readOnly {
		flags = os.O_RDONLY
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return nil, err
	}
	return &OSBackend{
		file: f,
		path: path,
	}, nil
}

func (o *OSBackend) ReadAt(p []byte, off int64) (int, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.file.ReadAt(p, off)
}

func (o *OSBackend) WriteAt(p []byte, off int64) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.file.WriteAt(p, off)
}

func (o *OSBackend) Sync() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.file.Sync()
}

func (o *OSBackend) Truncate(size int64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.file.Truncate(size)
}

func (o *OSBackend) Size() (int64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	stat, err := o.file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (o *OSBackend) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.file == nil {
		return nil
	}
	err := o.file.Close()
	o.file = nil
	return err
}
