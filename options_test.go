package cocodb

import (
	"encoding/binary"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/kv"
)

func TestStorageMemoryOverridesDatabasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-be-created.coco")
	db, err := Open(path, Storage(StorageMemory))
	if err != nil {
		t.Fatalf("Open memory database: %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		return tx.Bucket("cache").Put([]byte("key"), []byte("value"))
	}); err != nil {
		t.Fatalf("write memory database: %v", err)
	}
}

func TestMultiWriterAcrossHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi-writer.coco")
	db1, err := Open(path, MultiWriter(), WriterTimeout(time.Second))
	if err != nil {
		t.Fatalf("open writer one: %v", err)
	}
	defer db1.Close()
	db2, err := Open(path, MultiWriter(), WriterTimeout(time.Second))
	if err != nil {
		t.Fatalf("open writer two: %v", err)
	}
	defer db2.Close()

	var wg sync.WaitGroup
	for _, db := range []*DB{db1, db2} {
		for range 5 {
			wg.Add(1)
			go func(db *DB) {
				defer wg.Done()
				if err := db.Update(func(tx *Tx) error {
					_, err := tx.Bucket("counter").Increment([]byte("writes"), 1)
					return err
				}); err != nil {
					t.Errorf("multi-writer update: %v", err)
				}
			}(db)
		}
	}
	wg.Wait()

	reader, err := Open(path)
	if err != nil {
		t.Fatalf("open committed result: %v", err)
	}
	defer reader.Close()
	value, err := reader.Bucket("counter").Get([]byte("writes"))
	if err != nil || len(value) != 8 || int64(binary.BigEndian.Uint64(value)) != 10 {
		t.Fatalf("multi-writer total = %v, %v", value, err)
	}
}

func TestCustomProfileAndStorage(t *testing.T) {
	var calls []bool
	db, err := Open("custom",
		CustomProfile(ProfileConfig{
			MemoryLimit:   8 * 1024 * 1024,
			SyncMode:      SyncOff,
			Background:    false,
			CleanInterval: time.Millisecond,
		}),
		CustomStorage(func(_ string, _ bool, wal bool) (Backend, error) {
			calls = append(calls, wal)
			return file.NewMemoryBackend(), nil
		}),
	)
	if err != nil {
		t.Fatalf("Open custom storage: %v", err)
	}
	defer db.Close()

	if len(calls) != 2 || calls[0] || !calls[1] {
		t.Fatalf("custom storage factory calls = %v, want main then WAL", calls)
	}
	if db.opts.MemoryLimit != 8*1024*1024 || db.opts.SyncMode != SyncOff || db.scheduler != nil {
		t.Fatalf("custom profile was not applied: %+v", db.opts)
	}
}

func TestModelStorageSeparatesKVAndDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.coco")
	db, err := Open(path, DefaultDisk(), KVStorage(StorageMemory))
	if err != nil {
		t.Fatalf("Open mixed storage: %v", err)
	}
	if db.modelDB(ModelKV) == db || db.modelDB(ModelDocument) != db {
		t.Fatal("KV should use its own memory engine while documents use the disk default")
	}
	if err := db.Update(func(tx *Tx) error {
		if err := tx.Bucket("cache").Put([]byte("token"), []byte("ephemeral")); err != nil {
			return err
		}
		_, err := tx.Collection("users").Insert(Document{"_id": "u1", "name": "Ada"})
		return err
	}); err != nil {
		t.Fatalf("mixed write: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close mixed storage: %v", err)
	}

	db, err = Open(path, DefaultDisk(), KVStorage(StorageMemory))
	if err != nil {
		t.Fatalf("reopen mixed storage: %v", err)
	}
	defer db.Close()
	if _, err := db.Bucket("cache").Get([]byte("token")); err != kv.ErrNotFound {
		t.Fatalf("memory KV should not persist, got %v", err)
	}
	if _, err := db.Collection("users").Get("u1"); err != nil {
		t.Fatalf("disk document should persist: %v", err)
	}
}
