package cocodb

import (
	"path/filepath"
	"testing"
)

// New buckets used to share one B-tree because trees were cached by root page and every
// unwritten bucket has the root InvalidPageID.
func TestBucketsAreIsolated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buckets.coco")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// Two new buckets in one transaction, a third in a later one.
	if err := db.Update(func(tx *Tx) error {
		if err := tx.Bucket("one").Put([]byte("a"), []byte("1")); err != nil {
			return err
		}
		return tx.Bucket("two").Put([]byte("b"), []byte("2"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *Tx) error {
		return tx.Bucket("three").Put([]byte("c"), []byte("3"))
	}); err != nil {
		t.Fatal(err)
	}

	check := func(label string, db *DB) {
		t.Helper()
		want := map[string]string{"one": "a", "two": "b", "three": "c"}
		_ = db.View(func(tx *Tx) error {
			for bucket, key := range want {
				var keys []string
				it := tx.Bucket(bucket).Iterator()
				for ; it.Valid(); it.Next() {
					keys = append(keys, string(it.Key()))
				}
				_ = it.Close()
				if len(keys) != 1 || keys[0] != key {
					t.Errorf("%s: bucket %q holds keys %v, want [%s]", label, bucket, keys, key)
				}
			}
			return nil
		})
	}
	check("same process", db)

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	check("after reopen", db)

	// The non-transactional helper must also keep one tree per bucket.
	b := db.Bucket("four")
	if err := b.Put([]byte("d"), []byte("4")); err != nil {
		t.Fatal(err)
	}
	if v, err := db.Bucket("four").Get([]byte("d")); err != nil || string(v) != "4" {
		t.Errorf("db.Bucket(four) second handle: %q, %v", v, err)
	}
	if ok, _ := db.Bucket("one").Exists([]byte("d")); ok {
		t.Error(`bucket "one" sees a key written to bucket "four"`)
	}
}
