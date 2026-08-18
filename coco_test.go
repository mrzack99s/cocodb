package cocodb_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	coco "cocodb"
)

func TestCoCoEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "app.coco")

	// 1. Open Database
	db, err := coco.Open(dbPath,
		coco.Profile(coco.Balanced),
		coco.SyncMode(coco.SyncNormal),
	)
	if err != nil {
		t.Fatalf("coco.Open failed: %v", err)
	}

	// 2. Atomic Multi-Model Transaction (Document + KV)
	err = db.Update(func(tx *coco.Tx) error {
		users := tx.Collection("users")
		counters := tx.Bucket("counters")

		_, err := users.Insert(coco.Document{
			"_id":    "u100",
			"name":   "Alice",
			"age":    int64(30),
			"active": true,
			"tags":   []any{"go", "database"},
		})
		if err != nil {
			return err
		}

		_, err = counters.Increment([]byte("active-users"), 1)
		return err
	})
	if err != nil {
		t.Fatalf("Atomic Update failed: %v", err)
	}

	// 3. Read Snapshot in View
	err = db.View(func(tx *coco.Tx) error {
		users := tx.Collection("users")
		counters := tx.Bucket("counters")

		doc, err := users.Get("u100")
		if err != nil || doc["name"] != "Alice" {
			t.Fatalf("Get user failed: %v, doc=%v", err, doc)
		}

		val, err := counters.Get([]byte("active-users"))
		if err != nil {
			t.Fatalf("Get counter failed: %v", err)
		}
		_ = val
		return nil
	})
	if err != nil {
		t.Fatalf("View failed: %v", err)
	}

	// 4. Query Documents with Fluent API
	coll := db.Collection("users")
	rows, err := coll.Query().
		Where("active").Eq(true).
		Where("age").Gte(int64(18)).
		OrderBy("age", coco.Desc).
		Limit(10).
		All()
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Alice" {
		t.Fatalf("Query result mismatch: %v", rows)
	}

	// 5. Query Explain
	plan, err := coll.Query().Where("active").Eq(true).Explain()
	if err != nil || plan == "" {
		t.Fatalf("Explain failed: %v, plan=%s", err, plan)
	}

	// 6. Integrity Check
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	report, err := db.Check(ctx)
	if err != nil || !report.Valid {
		t.Fatalf("Integrity check failed: %v, errors=%v", err, report.Errors)
	}

	// 7. Point-in-time Backup
	backupPath := filepath.Join(tempDir, "backup.coco")
	if err := db.Backup(ctx, backupPath); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	// 8. Stats
	stats := db.Stats()
	if stats.PageCount < 2 {
		t.Fatalf("unexpected page count in stats: %d", stats.PageCount)
	}

	// 9. Close and Reopen
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	db2, err := coco.Open(dbPath, coco.Profile(coco.Balanced))
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer db2.Close()

	doc2, err := db2.Collection("users").Get("u100")
	if err != nil || doc2["name"] != "Alice" {
		t.Fatalf("Reopened document get mismatch: %v, doc=%v", err, doc2)
	}
}

func TestInMemoryMode(t *testing.T) {
	db, err := coco.Open(":memory:")
	if err != nil {
		t.Fatalf("Open in-memory failed: %v", err)
	}
	defer db.Close()

	err = db.Update(func(tx *coco.Tx) error {
		b := tx.Bucket("kv")
		return b.Put([]byte("key1"), []byte("value1"))
	})
	if err != nil {
		t.Fatalf("in-memory Put failed: %v", err)
	}

	err = db.View(func(tx *coco.Tx) error {
		b := tx.Bucket("kv")
		val, err := b.Get([]byte("key1"))
		if err != nil || string(val) != "value1" {
			t.Fatalf("in-memory Get failed: %v, val=%s", err, string(val))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("in-memory View failed: %v", err)
	}
}
