package wal_test

import (
	"bytes"
	"testing"

	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/wal"
)

func TestWALAppendAndRecovery(t *testing.T) {
	memDB := file.NewMemoryBackend()
	pager, err := storage.OpenPager(memDB, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	memWAL := file.NewMemoryBackend()
	walManager, err := wal.OpenWAL(memWAL, 0)
	if err != nil {
		t.Fatalf("OpenWAL failed: %v", err)
	}

	// 1. Transaction 1: writes page 2 and commits
	p2, _ := pager.Allocate(storage.PageRecord)
	sp2 := storage.WrapSlotted(p2)
	_, _ = sp2.Insert([]byte("committed txn 1 data"))
	p2.Seal()

	rec1 := wal.NewPageUpdateRecord(0, 1, p2.Header.ID, p2.Data)
	_, _ = walManager.Append(rec1)
	commit1 := wal.NewTxnCommitRecord(0, 1)
	_, _ = walManager.Append(commit1)

	// 2. Transaction 2: writes page 3 but aborts/crashes (no commit record)
	p3, _ := pager.Allocate(storage.PageRecord)
	sp3 := storage.WrapSlotted(p3)
	_, _ = sp3.Insert([]byte("uncommitted txn 2 data"))
	p3.Seal()

	rec2 := wal.NewPageUpdateRecord(0, 2, p3.Header.ID, p3.Data)
	_, _ = walManager.Append(rec2)
	// No commit for txn 2

	_ = walManager.Sync()

	// 3. Simulate Crash: create fresh pager with blank memory
	freshMemDB := file.NewMemoryBackend()
	freshPager, err := storage.OpenPager(freshMemDB, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("fresh OpenPager failed: %v", err)
	}

	// Run Recovery
	res, err := wal.Recover(walManager, freshPager)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	if res.TxnsCommitted != 1 {
		t.Fatalf("expected 1 committed txn, got %d", res.TxnsCommitted)
	}

	// Verify that page 2 has the committed data
	p2Recovered, err := freshPager.Get(2)
	if err != nil {
		t.Fatalf("failed to read recovered page 2: %v", err)
	}
	spRecovered := storage.WrapSlotted(p2Recovered)
	val, err := spRecovered.Get(0)
	if err != nil || !bytes.Equal(val, []byte("committed txn 1 data")) {
		t.Fatalf("recovered page 2 mismatch: %v, got %s", err, string(val))
	}
}
