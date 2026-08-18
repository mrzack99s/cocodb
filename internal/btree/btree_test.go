package btree_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/mrzack99s/cocodb/internal/btree"
	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/storage"
)

func TestKeyEncodingOrder(t *testing.T) {
	// Test Int64 ordering (-500 < -10 < 0 < 1 < 1000)
	ints := []int64{-100000, -500, -1, 0, 1, 42, 1000000}
	for i := 0; i < len(ints)-1; i++ {
		encA := btree.EncodeInt64(ints[i])
		encB := btree.EncodeInt64(ints[i+1])
		if bytes.Compare(encA, encB) >= 0 {
			t.Fatalf("int64 order failed: %d vs %d", ints[i], ints[i+1])
		}
		decA, err := btree.DecodeInt64(encA)
		if err != nil || decA != ints[i] {
			t.Fatalf("decode int64 failed: %v", err)
		}
	}

	// Test Float64 ordering (-10.5 < -0.1 < 0.0 < 0.1 < 99.9)
	floats := []float64{-100.5, -1.0, -0.001, 0.0, 0.0001, 10.5, 999.99}
	for i := 0; i < len(floats)-1; i++ {
		encA := btree.EncodeFloat64(floats[i])
		encB := btree.EncodeFloat64(floats[i+1])
		if bytes.Compare(encA, encB) >= 0 {
			t.Fatalf("float64 order failed: %f vs %f", floats[i], floats[i+1])
		}
		decA, err := btree.DecodeFloat64(encA)
		if err != nil || decA != floats[i] {
			t.Fatalf("decode float64 failed: %v", err)
		}
	}

	// Test String ordering
	strs := []string{"", "a", "aa", "ab", "b", "hello", "world"}
	for i := 0; i < len(strs)-1; i++ {
		encA := btree.EncodeString(strs[i])
		encB := btree.EncodeString(strs[i+1])
		if bytes.Compare(encA, encB) >= 0 {
			t.Fatalf("string order failed: %q vs %q", strs[i], strs[i+1])
		}
	}

	// Test Time ordering
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	encT1 := btree.EncodeTime(t1)
	encT2 := btree.EncodeTime(t2)
	if bytes.Compare(encT1, encT2) >= 0 {
		t.Fatalf("time order failed: %v vs %v", t1, t2)
	}
}

func TestBTreeRandomInsertAndCursor(t *testing.T) {
	mem := file.NewMemoryBackend()
	pager, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	tree := btree.NewBTree(pager, pager.Meta().CatalogRoot)

	refMap := make(map[string]string)
	keys := make([]string, 0)

	r := rand.New(rand.NewSource(42))
	n := 2000

	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key-%06d-%d", r.Intn(n*2), i)
		v := fmt.Sprintf("val-%d", i)
		if err := tree.Insert([]byte(k), []byte(v)); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
		refMap[k] = v
	}

	for k := range refMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Verify Search
	for _, k := range keys {
		val, found, err := tree.Search([]byte(k))
		if err != nil || !found {
			t.Fatalf("search %s failed: found=%v, err=%v", k, found, err)
		}
		if string(val) != refMap[k] {
			t.Fatalf("search %s mismatch: got %s, want %s", k, string(val), refMap[k])
		}
	}

	// Verify Tree Invariants
	if err := tree.VerifyTree(); err != nil {
		t.Fatalf("VerifyTree failed: %v", err)
	}

	// Forward Cursor scan
	cur := btree.NewCursor(tree)
	defer cur.Close()

	if !cur.First() {
		t.Fatalf("cursor.First failed: %v", cur.Err())
	}

	idx := 0
	for cur.Valid() {
		k := string(cur.Key())
		v := string(cur.Value())
		if k != keys[idx] {
			t.Fatalf("cursor forward mismatch at %d: got %s, want %s", idx, k, keys[idx])
		}
		if v != refMap[k] {
			t.Fatalf("cursor value mismatch at %d: got %s, want %s", idx, v, refMap[k])
		}
		idx++
		if !cur.Next() {
			break
		}
	}

	if idx != len(keys) {
		t.Fatalf("cursor walked %d items, want %d", idx, len(keys))
	}

	// Reverse Cursor scan
	if !cur.Last() {
		t.Fatalf("cursor.Last failed: %v", cur.Err())
	}
	revIdx := len(keys) - 1
	for cur.Valid() {
		k := string(cur.Key())
		if k != keys[revIdx] {
			t.Fatalf("cursor reverse mismatch at %d: got %s, want %s", revIdx, k, keys[revIdx])
		}
		revIdx--
		if !cur.Prev() {
			break
		}
	}
	if revIdx != -1 {
		t.Fatalf("cursor reverse did not reach beginning: %d", revIdx)
	}
}

func TestBTreeDeletions(t *testing.T) {
	mem := file.NewMemoryBackend()
	pager, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	tree := btree.NewBTree(pager, pager.Meta().CatalogRoot)

	// Insert 500 items
	for i := 0; i < 500; i++ {
		k := []byte(fmt.Sprintf("item-%04d", i))
		v := []byte(fmt.Sprintf("data-%d", i))
		_ = tree.Insert(k, v)
	}

	// Delete even items
	for i := 0; i < 500; i += 2 {
		k := []byte(fmt.Sprintf("item-%04d", i))
		if err := tree.Delete(k); err != nil {
			t.Fatalf("delete %s failed: %v", string(k), err)
		}
	}

	// Verify deleted items are gone, odd items remain
	for i := 0; i < 500; i++ {
		k := []byte(fmt.Sprintf("item-%04d", i))
		val, found, err := tree.Search(k)
		if err != nil {
			t.Fatalf("search %s error: %v", string(k), err)
		}
		if i%2 == 0 {
			if found {
				t.Fatalf("expected key %s to be deleted, but was found", string(k))
			}
		} else {
			if !found || string(val) != fmt.Sprintf("data-%d", i) {
				t.Fatalf("odd key %s not found or mismatch: %s", string(k), string(val))
			}
		}
	}

	if err := tree.VerifyTree(); err != nil {
		t.Fatalf("VerifyTree failed after deletions: %v", err)
	}
}

func TestBTreePrefixAndRangeScan(t *testing.T) {
	mem := file.NewMemoryBackend()
	pager, err := storage.OpenPager(mem, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	tree := btree.NewBTree(pager, pager.Meta().CatalogRoot)

	// Insert users and products
	for i := 0; i < 50; i++ {
		_ = tree.Insert([]byte(fmt.Sprintf("prod:%03d", i)), []byte("product"))
		_ = tree.Insert([]byte(fmt.Sprintf("user:%03d", i)), []byte("user"))
	}

	cur := btree.NewCursor(tree)
	defer cur.Close()

	var users []string
	err = cur.PrefixScan([]byte("user:"), func(k, v []byte) bool {
		users = append(users, string(k))
		return true
	})
	if err != nil {
		t.Fatalf("PrefixScan failed: %v", err)
	}
	if len(users) != 50 {
		t.Fatalf("expected 50 users from prefix scan, got %d", len(users))
	}

	var rangeItems []string
	err = cur.RangeScan([]byte("user:010"), []byte("user:020"), func(k, v []byte) bool {
		rangeItems = append(rangeItems, string(k))
		return true
	})
	if err != nil {
		t.Fatalf("RangeScan failed: %v", err)
	}
	if len(rangeItems) != 10 {
		t.Fatalf("expected 10 items in range [010, 020), got %d", len(rangeItems))
	}
}
