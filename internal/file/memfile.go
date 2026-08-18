package file

import (
	"io"
	"sync"
)

// MemoryBackend provides an in-memory Backend implementation.
type MemoryBackend struct {
	mu   sync.RWMutex
	buf  []byte
	open bool
}

// NewMemoryBackend creates a new in-memory file backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		buf:  make([]byte, 0),
		open: true,
	}
}

func (m *MemoryBackend) ReadAt(p []byte, off int64) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.open {
		return 0, io.ErrClosedPipe
	}
	if off < 0 {
		return 0, io.EOF
	}
	if off >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *MemoryBackend) WriteAt(p []byte, off int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.open {
		return 0, io.ErrClosedPipe
	}
	if off < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	requiredLen := int(off) + len(p)
	if requiredLen > len(m.buf) {
		newBuf := make([]byte, requiredLen)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *MemoryBackend) Sync() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.open {
		return io.ErrClosedPipe
	}
	return nil
}

func (m *MemoryBackend) Truncate(size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.open {
		return io.ErrClosedPipe
	}
	if size < 0 {
		size = 0
	}
	if int(size) < len(m.buf) {
		m.buf = m.buf[:size]
	} else if int(size) > len(m.buf) {
		newBuf := make([]byte, size)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	return nil
}

func (m *MemoryBackend) Size() (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.open {
		return 0, io.ErrClosedPipe
	}
	return int64(len(m.buf)), nil
}

func (m *MemoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.open = false
	return nil
}
