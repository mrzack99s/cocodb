package cocodb_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	coco "github.com/mrzack99s/cocodb"
)

func TestEngineLevelEncryptionEndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "secure_vault.coco")

	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i*7 + 42)
	}

	// 1. Create encrypted database
	db, err := coco.Open(dbPath, coco.EncryptionKey(masterKey))
	if err != nil {
		t.Fatalf("coco.Open with EncryptionKey failed: %v", err)
	}

	// Write Document and KV inside Update transaction
	err = db.Update(func(tx *coco.Tx) error {
		col := tx.Collection("classified_emails")
		_, err := col.Insert(coco.Document{
			"_id":     "msg-001",
			"subject": "Top Secret Project Apollo",
			"body":    "Extremely confidential project details and roadmap",
			"author":  "general@military.gov",
		})
		if err != nil {
			return err
		}

		bucket := tx.Bucket("secrets")
		return bucket.Put([]byte("master_token"), []byte("ultra_secret_api_token_12345"))
	})
	if err != nil {
		t.Fatalf("db.Update failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("db.Close failed: %v", err)
	}

	// 2. Inspect raw bytes on disk
	rawDBBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("reading raw db file failed: %v", err)
	}

	// Assert: No unencrypted magic header
	if bytes.Contains(rawDBBytes, []byte("COCODB")) {
		t.Fatalf("SECURITY BREACH: raw database file contains unencrypted magic header 'COCODB'")
	}
	// Assert: No unencrypted collection names or document fields
	if bytes.Contains(rawDBBytes, []byte("classified_emails")) {
		t.Fatalf("SECURITY BREACH: raw database file contains collection name 'classified_emails'")
	}
	if bytes.Contains(rawDBBytes, []byte("Top Secret Project Apollo")) {
		t.Fatalf("SECURITY BREACH: raw database file contains plaintext subject")
	}
	if bytes.Contains(rawDBBytes, []byte("ultra_secret_api_token_12345")) {
		t.Fatalf("SECURITY BREACH: raw database file contains plaintext KV value")
	}

	// 3. Attempt opening with WRONG key -> must FAIL CLOSED
	wrongKey := make([]byte, 32)
	copy(wrongKey, masterKey)
	wrongKey[0] ^= 0xFF

	_, errWrong := coco.Open(dbPath, coco.EncryptionKey(wrongKey))
	if errWrong == nil {
		t.Fatalf("SECURITY FAILURE: expected Open with WRONG key to fail closed, but got nil error")
	}

	// 4. Attempt opening with NO key -> must FAIL CLOSED
	_, errNoKey := coco.Open(dbPath)
	if errNoKey == nil {
		t.Fatalf("SECURITY FAILURE: expected Open with NO key to fail closed, but got nil error")
	}

	// 5. Open with CORRECT key -> must succeed and retrieve all data accurately
	dbCorrect, err := coco.Open(dbPath, coco.EncryptionKey(masterKey))
	if err != nil {
		t.Fatalf("failed to open database with correct key: %v", err)
	}
	defer dbCorrect.Close()

	// Read Document & KV inside View transaction
	err = dbCorrect.View(func(tx *coco.Tx) error {
		col := tx.Collection("classified_emails")
		doc, err := col.Get("msg-001")
		if err != nil {
			t.Fatalf("col.Get failed: %v", err)
		}
		if doc == nil || doc["subject"] != "Top Secret Project Apollo" {
			t.Fatalf("unexpected document data retrieved: %v", doc)
		}
		if doc["body"] != "Extremely confidential project details and roadmap" {
			t.Fatalf("unexpected document body retrieved: %v", doc)
		}

		bucket := tx.Bucket("secrets")
		val, err := bucket.Get([]byte("master_token"))
		if err != nil {
			t.Fatalf("bucket.Get failed: %v", err)
		}
		if string(val) != "ultra_secret_api_token_12345" {
			t.Fatalf("bucket.Get value mismatch: got %q", string(val))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dbCorrect.View failed: %v", err)
	}
}

func TestRecordIDPersistenceAcrossReopen(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_reopen.coco")

	// 1. Open DB, insert user document
	db, err := coco.Open(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	err = db.Update(func(tx *coco.Tx) error {
		users := tx.Collection("users")
		_, err := users.Insert(coco.Document{
			"_id":      "user-1",
			"username": "admin",
			"role":     "admin",
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert user failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db failed: %v", err)
	}

	// 2. Reopen DB, insert folder documents in another collection
	db2, err := coco.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	err = db2.Update(func(tx *coco.Tx) error {
		folders := tx.Collection("folders")
		for i := 1; i <= 5; i++ {
			_, err := folders.Insert(coco.Document{
				"_id":  filepath.Join("folder-", string(rune('0'+i))),
				"name": "Inbox",
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert folders failed: %v", err)
	}

	// 3. Verify user document is still untouched and retrievable without collision!
	err = db2.View(func(tx *coco.Tx) error {
		users := tx.Collection("users")
		doc, err := users.Get("user-1")
		if err != nil {
			t.Fatalf("users.Get failed: %v", err)
		}
		if doc == nil || doc["username"] != "admin" {
			t.Fatalf("user doc corrupted or collided: got %v", doc)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("view failed: %v", err)
	}
	_ = db2.Close()
}

