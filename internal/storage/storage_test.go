package storage_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/types"
)

func TestMemoryBackend(t *testing.T) {
	mem := file.NewMemoryBackend()
	defer mem.Close()

	data := []byte("hello cocodb storage kernel")
	n, err := mem.WriteAt(data, 100)
	if err != nil || n != len(data) {
		t.Fatalf("WriteAt failed: %v", err)
	}

	buf := make([]byte, len(data))
	n, err = mem.ReadAt(buf, 100)
	if err != nil || n != len(data) {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("mismatch: got %s, want %s", string(buf), string(data))
	}
}

func TestPagerAndDualMeta(t *testing.T) {
	mem := file.NewMemoryBackend()
	pager, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	meta := pager.Meta()
	if meta.Generation != 1 {
		t.Fatalf("expected gen 1, got %d", meta.Generation)
	}

	// Allocate pages
	p1, err := pager.Allocate(storage.PageRecord)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if p1.Header.ID != 2 {
		t.Fatalf("expected page 2, got %d", p1.Header.ID)
	}

	// Write data to p1 via slotted interface
	sp := storage.WrapSlotted(p1)
	slot0, err := sp.Insert([]byte("record data 1"))
	if err != nil {
		t.Fatalf("sp.Insert failed: %v", err)
	}

	val, err := sp.Get(slot0)
	if err != nil || string(val) != "record data 1" {
		t.Fatalf("sp.Get mismatch: %v, got %q", err, string(val))
	}

	pager.MarkDirty(p1)
	if err := pager.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}

	if pager.Meta().Generation < 2 {
		t.Fatalf("expected gen increment, got %d", pager.Meta().Generation)
	}

	// Reopen pager on same backend
	pager2, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}

	p1Reloaded, err := pager2.Get(2)
	if err != nil {
		t.Fatalf("failed to read page 2: %v", err)
	}
	sp2 := storage.WrapSlotted(p1Reloaded)
	val2, err := sp2.Get(slot0)
	if err != nil || string(val2) != "record data 1" {
		t.Fatalf("reloaded data mismatch: %v, got %q", err, string(val2))
	}
}

func TestSlottedPageCompaction(t *testing.T) {
	page := storage.NewPage(10, storage.PageRecord)
	sp := storage.WrapSlotted(page)

	var slots []types.SlotID
	for i := 0; i < 20; i++ {
		slot, err := sp.Insert([]byte(fmt.Sprintf("item-%03d-data-payload", i)))
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
		slots = append(slots, slot)
	}

	// Delete even slots
	for i := 0; i < len(slots); i += 2 {
		if err := sp.Delete(slots[i]); err != nil {
			t.Fatalf("delete %d failed: %v", slots[i], err)
		}
	}

	// Compact
	sp.Compact()

	// Verify odd slots are still intact
	for i := 1; i < len(slots); i += 2 {
		val, err := sp.Get(slots[i])
		if err != nil {
			t.Fatalf("get slot %d failed: %v", slots[i], err)
		}
		want := fmt.Sprintf("item-%03d-data-payload", i)
		if string(val) != want {
			t.Fatalf("slot %d: got %s, want %s", slots[i], string(val), want)
		}
	}
}

func TestPageAllocatorAndFreeList(t *testing.T) {
	mem := file.NewMemoryBackend()
	pager, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	// Allocate 100 pages
	allocated := make([]types.PageID, 100)
	for i := 0; i < 100; i++ {
		p, err := pager.Allocate(storage.PageRecord)
		if err != nil {
			t.Fatalf("allocate %d failed: %v", i, err)
		}
		allocated[i] = p.Header.ID
	}

	// Free 50 pages
	for i := 0; i < 50; i++ {
		if err := pager.Free(allocated[i]); err != nil {
			t.Fatalf("free %d failed: %v", allocated[i], err)
		}
	}

	// Allocate 50 pages again -> should reuse from freelist
	reused := make([]types.PageID, 50)
	for i := 0; i < 50; i++ {
		p, err := pager.Allocate(storage.PageRecord)
		if err != nil {
			t.Fatalf("re-allocate %d failed: %v", i, err)
		}
		reused[i] = p.Header.ID
	}

	if err := pager.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}
}

func TestOSBackendFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.coco")

	osb, err := file.OpenOSBackend(path, false)
	if err != nil {
		t.Fatalf("OpenOSBackend failed: %v", err)
	}
	defer osb.Close()

	pager, err := storage.OpenPager(osb, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager on OSBackend failed: %v", err)
	}

	p, err := pager.Allocate(storage.PageRecord)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	sp := storage.WrapSlotted(p)
	_, err = sp.Insert([]byte("persistent slotted record"))
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	pager.MarkDirty(p)
	if err := pager.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}
}
