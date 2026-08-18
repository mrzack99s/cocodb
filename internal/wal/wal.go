package wal

import (
	"bytes"
	"io"
	"sync"

	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/types"
)

// WAL manages write-ahead logging to guarantee ACID durability and crash safety.
type WAL struct {
	mu         sync.RWMutex
	backend    file.Backend
	lastLSN    types.LSN
	durableLSN types.LSN
	offset     int64
}

// OpenWAL opens or creates a WAL manager on top of a file backend.
func OpenWAL(backend file.Backend, lastLSN types.LSN) (*WAL, error) {
	size, err := backend.Size()
	if err != nil {
		return nil, err
	}

	w := &WAL{
		backend:    backend,
		lastLSN:    lastLSN,
		durableLSN: lastLSN,
		offset:     size,
	}
	return w, nil
}

// NextLSN increments and returns the next monotonic LSN.
func (w *WAL) NextLSN() types.LSN {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastLSN++
	return w.lastLSN
}

// LastLSN returns the latest assigned LSN.
func (w *WAL) LastLSN() types.LSN {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastLSN
}

// DurableLSN returns the highest LSN successfully flushed to stable storage.
func (w *WAL) DurableLSN() types.LSN {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.durableLSN
}

// Append writes a WAL record to the log.
func (w *WAL) Append(rec *Record) (types.LSN, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lastLSN++
	rec.Header.LSN = w.lastLSN

	data := rec.Encode()
	if _, err := w.backend.WriteAt(data, w.offset); err != nil {
		return 0, err
	}
	w.offset += int64(len(data))
	return rec.Header.LSN, nil
}

// Flush ensures that all WAL records up to targetLSN are synced to disk.
func (w *WAL) Flush(targetLSN types.LSN) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if targetLSN <= w.durableLSN {
		return nil
	}

	if err := w.backend.Sync(); err != nil {
		return err
	}
	w.durableLSN = w.lastLSN
	return nil
}

// Sync flushes all pending WAL writes to stable storage.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.backend.Sync(); err != nil {
		return err
	}
	w.durableLSN = w.lastLSN
	return nil
}

// Truncate clears the WAL after a successful checkpoint.
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.backend.Truncate(0); err != nil {
		return err
	}
	w.offset = 0
	return nil
}

// ReadAll records scans the entire WAL backend and returns all valid decoded records.
func (w *WAL) ReadAll() ([]*Record, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	size, err := w.backend.Size()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	buf := make([]byte, size)
	if _, err := w.backend.ReadAt(buf, 0); err != nil && err != io.EOF {
		return nil, err
	}

	reader := bytes.NewReader(buf)
	var records []*Record

	for {
		rec, err := DecodeRecord(reader)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			// Torn or invalid record at end of WAL
			break
		}
		records = append(records, rec)
	}

	return records, nil
}

// Close closes the WAL backend.
func (w *WAL) Close() error {
	_ = w.Sync()
	return w.backend.Close()
}
