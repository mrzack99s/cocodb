package txn_test

import (
	"bytes"
	"testing"

	"cocodb/internal/file"
	"cocodb/internal/record"
	"cocodb/internal/storage"
	"cocodb/internal/txn"
	"cocodb/internal/wal"
)

func TestSnapshotIsolationAndVersionChains(t *testing.T) {
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

	tm := txn.NewTxnManager(pager, walManager, txn.SyncOff)
	dir := record.NewDirectory(pager, pager.Meta().RecordDirRoot)
	store := record.NewStore(pager, dir, tm)

	// 1. Transaction 1: Insert initial record
	tx1, err := tm.Begin(false)
	if err != nil {
		t.Fatalf("Begin tx1 failed: %v", err)
	}

	recID1, err := store.WriteRecord(tx1, []byte("version 1 payload"), 0)
	if err != nil {
		t.Fatalf("WriteRecord failed: %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("Commit tx1 failed: %v", err)
	}

	// 2. Reader R1 opens snapshot
	r1, err := tm.Begin(true)
	if err != nil {
		t.Fatalf("Begin r1 failed: %v", err)
	}
	defer r1.Rollback()

	// 3. Writer W updates record to Version 2
	w, err := tm.Begin(false)
	if err != nil {
		t.Fatalf("Begin w failed: %v", err)
	}

	recID2, err := store.WriteRecord(w, []byte("version 2 payload"), recID1)
	if err != nil {
		t.Fatalf("WriteRecord v2 failed: %v", err)
	}
	if err := w.Commit(); err != nil {
		t.Fatalf("Commit w failed: %v", err)
	}

	// 4. Reader R2 opens snapshot after update
	r2, err := tm.Begin(true)
	if err != nil {
		t.Fatalf("Begin r2 failed: %v", err)
	}
	defer r2.Rollback()

	// Verify R1 sees Version 1
	_, valR1, err := store.ReadRecord(r1, recID2)
	if err != nil {
		t.Fatalf("r1 ReadRecord failed: %v", err)
	}
	if string(valR1) != "version 1 payload" {
		t.Fatalf("r1 expected 'version 1 payload', got %q", string(valR1))
	}

	// Verify R2 sees Version 2
	_, valR2, err := store.ReadRecord(r2, recID2)
	if err != nil {
		t.Fatalf("r2 ReadRecord failed: %v", err)
	}
	if string(valR2) != "version 2 payload" {
		t.Fatalf("r2 expected 'version 2 payload', got %q", string(valR2))
	}
}

func TestLargeRecordOverflow(t *testing.T) {
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

	tm := txn.NewTxnManager(pager, walManager, txn.SyncOff)
	dir := record.NewDirectory(pager, pager.Meta().RecordDirRoot)
	store := record.NewStore(pager, dir, tm)

	tx, err := tm.Begin(false)
	if err != nil {
		t.Fatalf("Begin tx failed: %v", err)
	}

	// 50 KB payload (spans multiple 16 KB overflow pages)
	bigData := make([]byte, 50*1024)
	for i := range bigData {
		bigData[i] = byte(i % 251)
	}

	recID, err := store.WriteRecord(tx, bigData, 0)
	if err != nil {
		t.Fatalf("WriteRecord bigData failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Read back
	rTx, err := tm.Begin(true)
	if err != nil {
		t.Fatalf("Begin read tx failed: %v", err)
	}
	defer rTx.Rollback()

	_, readData, err := store.ReadRecord(rTx, recID)
	if err != nil {
		t.Fatalf("ReadRecord bigData failed: %v", err)
	}
	if !bytes.Equal(readData, bigData) {
		t.Fatalf("large record payload corrupted or truncated: got %d bytes, want %d", len(readData), len(bigData))
	}
}

func TestSavepoints(t *testing.T) {
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

	tm := txn.NewTxnManager(pager, walManager, txn.SyncOff)
	dir := record.NewDirectory(pager, pager.Meta().RecordDirRoot)
	store := record.NewStore(pager, dir, tm)

	tx, err := tm.Begin(false)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	recID1, _ := store.WriteRecord(tx, []byte("initial"), 0)

	// Savepoint A
	if err := tx.Savepoint("sp1"); err != nil {
		t.Fatalf("Savepoint failed: %v", err)
	}

	// Overwrite to bad state
	_, _ = store.WriteRecord(tx, []byte("bad mutation"), recID1)

	// Rollback to Savepoint A
	if err := tx.RollbackTo("sp1"); err != nil {
		t.Fatalf("RollbackTo failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
