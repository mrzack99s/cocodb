package backup

import (
	"context"
	"fmt"
	"io"
	"os"

	"cocodb/internal/file"
	"cocodb/internal/storage"
	"cocodb/internal/types"
)

// Backup creates a point-in-time backup snapshot file at dstPath.
func Backup(ctx context.Context, pager storage.Pager, dstPath string) error {
	if err := pager.FlushAll(); err != nil {
		return err
	}

	size, err := pager.Backend().Size()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	buf := make([]byte, types.DefaultPageSize)
	var offset int64
	for offset < size {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := pager.Backend().ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		if _, err := dstFile.WriteAt(buf[:n], offset); err != nil {
			return err
		}
		offset += int64(n)
	}

	return dstFile.Sync()
}

// Restore restores a backup from srcPath to dstPath after verifying integrity.
func Restore(srcPath, dstPath string) error {
	srcBackend, err := file.OpenOSBackend(srcPath, true)
	if err != nil {
		return err
	}
	defer srcBackend.Close()

	// Verify meta pages
	pager, err := storage.OpenPager(srcBackend, 16*1024*1024, true)
	if err != nil {
		return fmt.Errorf("invalid backup source: %w", err)
	}
	_ = pager.Close()

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	return dstFile.Sync()
}
