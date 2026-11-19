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

func TestEncryptedWALAndRecovery(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 0x33)
	}

	memDB := file.NewMemoryBackend()
	pager, err := storage.OpenPager(memDB, 16*1024*1024, false, key)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	memWAL := file.NewMemoryBackend()
	walManager, err := wal.OpenWAL(memWAL, 0, key)
	if err != nil {
		t.Fatalf("OpenWAL failed: %v", err)
	}

	p2, _ := pager.Allocate(storage.PageRecord)
	sp2 := storage.WrapSlotted(p2)
	_, _ = sp2.Insert([]byte("sensitive encrypted wal transaction data"))
	p2.Seal()

	rec1 := wal.NewPageUpdateRecord(0, 1, p2.Header.ID, p2.Data)
	_, err = walManager.Append(rec1)
	if err != nil {
		t.Fatalf("Append rec1 failed: %v", err)
	}
	commit1 := wal.NewTxnCommitRecord(0, 1)
	_, err = walManager.Append(commit1)
	if err != nil {
		t.Fatalf("Append commit1 failed: %v", err)
	}

	_ = walManager.Sync()

	// 1. Check raw WAL bytes on backend: MUST NOT contain plaintext
	rawWAL := make([]byte, 20000)
	n, _ := memWAL.ReadAt(rawWAL, 0)
	if bytes.Contains(rawWAL[:n], []byte("sensitive encrypted wal transaction data")) {
		t.Fatalf("raw WAL MUST NOT contain plaintext transaction data")
	}

	// 2. Read with WRONG key -> must not decode unauthenticated records
	wrongKey := make([]byte, 32)
	wrongKey[0] = 0x99
	walWrong, err := wal.OpenWAL(memWAL, 0, wrongKey)
	if err != nil {
		t.Fatalf("OpenWAL with wrong key: %v", err)
	}
	recordsWrong, _ := walWrong.ReadAll()
	if len(recordsWrong) != 0 {
		t.Fatalf("expected 0 decoded records with WRONG key, got %d", len(recordsWrong))
	}

	// 3. Read with CORRECT key -> must return 2 valid records
	walCorrect, err := wal.OpenWAL(memWAL, 0, key)
	if err != nil {
		t.Fatalf("OpenWAL with correct key: %v", err)
	}
	recordsCorrect, err := walCorrect.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll with correct key failed: %v", err)
	}
	if len(recordsCorrect) != 2 {
		t.Fatalf("expected 2 decoded records with correct key, got %d", len(recordsCorrect))
	}

	// 4. Test crash recovery on fresh encrypted pager
	freshMemDB := file.NewMemoryBackend()
	freshPager, err := storage.OpenPager(freshMemDB, 16*1024*1024, false, key)
	if err != nil {
		t.Fatalf("fresh OpenPager failed: %v", err)
	}

	res, err := wal.Recover(walCorrect, freshPager)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if res.TxnsCommitted != 1 {
		t.Fatalf("expected 1 committed txn, got %d", res.TxnsCommitted)
	}

	p2Recovered, err := freshPager.Get(2)
	if err != nil {
		t.Fatalf("failed to read recovered page 2: %v", err)
	}
	spRecovered := storage.WrapSlotted(p2Recovered)
	val, err := spRecovered.Get(0)
	if err != nil || !bytes.Equal(val, []byte("sensitive encrypted wal transaction data")) {
		t.Fatalf("recovered page 2 mismatch: %v, got %s", err, string(val))
	}
}

