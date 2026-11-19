package cocodb

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

var errAbort = errors.New("abort")

func docIDs(t *testing.T, db *DB, coll string) []string {
	t.Helper()
	var ids []string
	if err := db.View(func(tx *Tx) error {
		docs, err := tx.Collection(coll).Query().All()
		for _, d := range docs {
			ids = append(ids, fmt.Sprint(d["_id"]))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids
}

func bucketKeys(t *testing.T, db *DB, bucket string) []string {
	t.Helper()
	var keys []string
	_ = db.View(func(tx *Tx) error {
		it := tx.Bucket(bucket).Iterator()
		defer it.Close()
		for ; it.Valid(); it.Next() {
			keys = append(keys, string(it.Key()))
		}
		return nil
	})
	return keys
}

// A failed Update must leave no trace: not in later reads of the same process, not after
// a reopen, and later commits must work normally.
func TestRollbackLeavesNoTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.coco")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *Tx) error {
		if _, err := tx.Collection("things").Insert(Document{"_id": "1", "n": int64(1)}); err != nil {
			return err
		}
		return tx.Bucket("kv").Put([]byte("a"), []byte("1"))
	}); err != nil {
		t.Fatal(err)
	}

	// Abort a transaction that writes to an existing collection, a new collection, an
	// existing bucket and a new bucket, with enough data to split B+Tree pages.
	err = db.Update(func(tx *Tx) error {
		for i := 0; i < 300; i++ {
			id := fmt.Sprintf("tmp-%03d", i)
			if _, err := tx.Collection("things").Insert(Document{"_id": id, "pad": strings.Repeat("x", 200)}); err != nil {
				return err
			}
		}
		if _, err := tx.Collection("ghost").Insert(Document{"_id": "g"}); err != nil {
			return err
		}
		if err := tx.Bucket("kv").Put([]byte("b"), []byte("2")); err != nil {
			return err
		}
		if err := tx.Bucket("ghost-kv").Put([]byte("z"), []byte("9")); err != nil {
			return err
		}
		if err := tx.Collection("things").Replace("1", Document{"n": int64(99)}); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatalf("Update returned %v", err)
	}

	check := func(label string, db *DB) {
		t.Helper()
		if ids := docIDs(t, db, "things"); len(ids) != 1 || ids[0] != "1" {
			t.Errorf("%s: things = %v, want [1]", label, ids)
		}
		_ = db.View(func(tx *Tx) error {
			d, err := tx.Collection("things").Get("1")
			if err != nil || d["n"] != int64(1) {
				t.Errorf("%s: things/1 = %v, %v; want n=1", label, d, err)
			}
			return nil
		})
		if ids := docIDs(t, db, "ghost"); len(ids) != 0 {
			t.Errorf("%s: ghost = %v, want none", label, ids)
		}
		if keys := bucketKeys(t, db, "kv"); len(keys) != 1 || keys[0] != "a" {
			t.Errorf("%s: kv keys = %v, want [a]", label, keys)
		}
		if keys := bucketKeys(t, db, "ghost-kv"); len(keys) != 0 {
			t.Errorf("%s: ghost-kv keys = %v, want none", label, keys)
		}
	}
	check("same process", db)

	// The database keeps working after the rollback.
	if err := db.Update(func(tx *Tx) error {
		if _, err := tx.Collection("things").Insert(Document{"_id": "2"}); err != nil {
			return err
		}
		return tx.Bucket("kv").Put([]byte("c"), []byte("3"))
	}); err != nil {
		t.Fatal(err)
	}
	if ids := docIDs(t, db, "things"); len(ids) != 2 {
		t.Errorf("after a later commit: things = %v, want [1 2]", ids)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if ids := docIDs(t, db, "things"); len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Errorf("after reopen: things = %v, want [1 2]", ids)
	}
	if keys := bucketKeys(t, db, "kv"); len(keys) != 2 {
		t.Errorf("after reopen: kv keys = %v, want [a c]", keys)
	}
	if ids := docIDs(t, db, "ghost"); len(ids) != 0 {
		t.Errorf("after reopen: ghost = %v, want none", ids)
	}
}

// Several aborted transactions in a row, interleaved with commits.
func TestRepeatedRollbacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repeat.coco")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for round := 0; round < 20; round++ {
		commit := round%3 == 0
		err := db.Update(func(tx *Tx) error {
			for i := 0; i < 25; i++ {
				if _, err := tx.Collection("c").Insert(Document{"_id": fmt.Sprintf("r%02d-%02d", round, i)}); err != nil {
					return err
				}
			}
			if !commit {
				return errAbort
			}
			return nil
		})
		if commit {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
			want += 25
		}
		if got := len(docIDs(t, db, "c")); got != want {
			t.Fatalf("round %d (commit=%v): %d documents, want %d", round, commit, got, want)
		}
	}
	_ = db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := len(docIDs(t, db, "c")); got != want {
		t.Fatalf("after reopen: %d documents, want %d", got, want)
	}
}
