package document_test

import (
	"testing"

	"github.com/mrzack99s/cocodb/document"
	"github.com/mrzack99s/cocodb/internal/cson"
	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/index"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/internal/wal"
)

func TestCSONEncodingAndDocumentView(t *testing.T) {
	doc := map[string]any{
		"_id":       "u100",
		"name":      "Alice",
		"age":       int64(30),
		"active":    true,
		"score":     98.5,
		"embedding": []float32{0.1, 0.2, 0.3, 0.4},
	}

	dict := cson.NewFieldDictionary()
	data, err := cson.Encode(doc, dict)
	if err != nil {
		t.Fatalf("CSON Encode failed: %v", err)
	}

	view, err := cson.NewDocumentView(data, dict)
	if err != nil {
		t.Fatalf("NewDocumentView failed: %v", err)
	}

	name, ok := view.String("name")
	if !ok || name != "Alice" {
		t.Fatalf("view.String failed: %s, %v", name, ok)
	}

	age, ok := view.Int64("age")
	if !ok || age != 30 {
		t.Fatalf("view.Int64 failed: %d, %v", age, ok)
	}

	active, ok := view.Bool("active")
	if !ok || !active {
		t.Fatalf("view.Bool failed: %v, %v", active, ok)
	}

	vec, ok := view.Vector("embedding")
	if !ok || len(vec) != 4 || vec[0] != 0.1 {
		t.Fatalf("view.Vector failed: %v, %v", vec, ok)
	}
}

func TestCollectionCRUDAndSecondaryIndexes(t *testing.T) {
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
	dir := record.NewDirectory(pager, types.InvalidPageID)
	store := record.NewStore(pager, dir, tm)

	tx, _ := tm.Begin(false)

	coll := document.NewCollection("users", 1, pager, tx, store, nil, types.InvalidPageID)

	// Create a unique secondary index on "email"
	emailIdx := index.NewSecondaryIndex(index.IndexDefinition{
		Name:   "idx_email",
		Fields: []string{"email"},
		Unique: true,
	}, pager, types.InvalidPageID)
	coll.AddSecondaryIndex(emailIdx)

	// 1. Insert doc
	id, err := coll.Insert(document.Document{
		"_id":    "u1",
		"name":   "Alice",
		"email":  "alice@example.com",
		"age":    int64(25),
		"tags":   []any{"go", "database"},
		"active": true,
	})
	if err != nil || id != "u1" {
		t.Fatalf("Insert failed: %v, id=%s", err, id)
	}

	// 2. Test unique index conflict
	_, err = coll.Insert(document.Document{
		"_id":   "u2",
		"name":  "Alice Duplicate",
		"email": "alice@example.com",
	})
	if err == nil {
		t.Fatalf("expected unique index conflict on duplicate email")
	}

	// 3. Get document
	doc, err := coll.Get("u1")
	if err != nil || doc["name"] != "Alice" {
		t.Fatalf("Get failed: %v, %v", err, doc)
	}

	// 4. Update (Increment age, push tag)
	err = coll.Update("u1",
		document.Increment("age", 1),
		document.Push("tags", "nosql"),
	)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, err := coll.Get("u1")
	if err != nil || updated["age"] != int64(26) {
		t.Fatalf("Updated doc age mismatch: %v", updated)
	}

	// 5. Delete
	if err := coll.Delete("u1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = coll.Get("u1")
	if err == nil {
		t.Fatalf("expected document to be deleted")
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
