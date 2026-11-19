package cocodb

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests kill a child process (this test binary, re-run as a helper) at a chosen
// point and then reopen its database, the way a crash or power loss would leave it.

// runAndKill starts the helper in mode, waits until it prints "ready" and kills it.
func runAndKill(t *testing.T, path, mode string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
	cmd.Env = append(os.Environ(), "COCO_CRASH_PATH="+path, "COCO_CRASH_MODE="+mode)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(out)
		for s.Scan() {
			if line := s.Text(); line == "ready" || strings.HasPrefix(line, "error:") {
				ready <- line
				return
			}
		}
		ready <- "helper exited early"
	}()
	select {
	case msg := <-ready:
		if msg != "ready" {
			_ = cmd.Process.Kill()
			t.Fatal(msg)
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper never became ready")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
}

func countDocs(t *testing.T, db *DB, coll string) int {
	t.Helper()
	n := 0
	if err := db.View(func(tx *Tx) error {
		docs, err := tx.Collection(coll).Query().All()
		n = len(docs)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func reopen(t *testing.T, path string) *DB {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	return db
}

// fastCheckpoints runs background checkpoints every few milliseconds, so they interleave
// with the helper's transactions.
var fastCheckpoints = CustomProfile(ProfileConfig{
	MemoryLimit:   64 << 20,
	SyncMode:      SyncNormal,
	Background:    true,
	CleanInterval: 2 * time.Millisecond,
})

// A committed Update survives the process being killed before Close.
func TestCommitSurvivesKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.coco")
	runAndKill(t, path, "commit-one")
	db := reopen(t, path)
	defer db.Close()
	_ = db.View(func(tx *Tx) error {
		for i := 0; i < 50; i++ {
			id := fmt.Sprintf("doc-%02d", i)
			if d, err := tx.Collection("things").Get(id); err != nil || d["n"] != int64(i) {
				t.Errorf("document %s after kill: %v, %v", id, d, err)
				return nil
			}
		}
		if v, err := tx.Bucket("kv").Get([]byte("k")); err != nil || string(v) != "v" {
			t.Errorf("bucket key after kill: %q, %v", v, err)
		}
		return nil
	})
}

// Many commits with background checkpoints running between them, then a kill.
func TestManyCommitsWithCheckpointsSurviveKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.coco")
	runAndKill(t, path, "commit-many")
	db := reopen(t, path)
	defer db.Close()
	if n := countDocs(t, db, "things"); n != 40*25 {
		t.Fatalf("%d documents after kill, want %d", n, 40*25)
	}
}

// A transaction that has written but not committed when the process dies leaves no
// trace, even with checkpoints running while it was open; the commits before it are
// intact and the database keeps working.
func TestUncommittedWorkIsDiscardedOnKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.coco")
	runAndKill(t, path, "kill-mid-transaction")
	db := reopen(t, path)
	if n := countDocs(t, db, "things"); n != 50 {
		t.Fatalf("%d documents after kill, want the 50 committed ones", n)
	}
	if n := countDocs(t, db, "pending"); n != 0 {
		t.Fatalf("%d documents of the uncommitted transaction survived", n)
	}
	if err := db.Update(func(tx *Tx) error {
		_, err := tx.Collection("things").Insert(Document{"_id": "after-recovery"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = reopen(t, path)
	defer db.Close()
	if n := countDocs(t, db, "things"); n != 51 {
		t.Fatalf("%d documents after a write and clean reopen, want 51", n)
	}
}

// A rolled-back transaction before the kill stays rolled back after recovery.
func TestRollbackThenKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.coco")
	runAndKill(t, path, "rollback-then-hang")
	db := reopen(t, path)
	defer db.Close()
	if n := countDocs(t, db, "things"); n != 50 {
		t.Fatalf("%d documents, want the 50 committed ones", n)
	}
	if n := countDocs(t, db, "ghost"); n != 0 {
		t.Fatalf("%d documents of the rolled-back transaction survived", n)
	}
}

// Recover, write, get killed again, recover again.
func TestRepeatedCrashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.coco")
	for round := 1; round <= 3; round++ {
		runAndKill(t, path, "commit-one-more")
		db := reopen(t, path)
		if n := countDocs(t, db, "things"); n != round*10 {
			t.Fatalf("round %d: %d documents, want %d", round, n, round*10)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCrashHelper runs only as the child process of the tests above.
func TestCrashHelper(t *testing.T) {
	path, mode := os.Getenv("COCO_CRASH_PATH"), os.Getenv("COCO_CRASH_MODE")
	if path == "" {
		t.Skip("helper process only")
	}
	fail := func(err error) {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(2)
	}
	open := func(opts ...Option) *DB {
		db, err := Open(path, opts...)
		if err != nil {
			fail(err)
		}
		return db
	}
	insert := func(db *DB, coll, prefix string, n int) {
		if err := db.Update(func(tx *Tx) error {
			for i := 0; i < n; i++ {
				if _, err := tx.Collection(coll).Insert(Document{"_id": fmt.Sprintf("%s-%02d", prefix, i), "n": int64(i)}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			fail(err)
		}
	}
	hang := func() {
		os.Stdout.WriteString("ready\n")
		time.Sleep(2 * time.Minute) // killed by the parent long before this
		os.Exit(3)
	}

	switch mode {
	case "commit-one":
		db := open(Background(false))
		insert(db, "things", "doc", 50)
		if err := db.Update(func(tx *Tx) error { return tx.Bucket("kv").Put([]byte("k"), []byte("v")) }); err != nil {
			fail(err)
		}
		hang()
	case "commit-many":
		db := open(fastCheckpoints)
		for round := 0; round < 40; round++ {
			insert(db, "things", fmt.Sprintf("r%02d", round), 25)
			time.Sleep(3 * time.Millisecond) // let checkpoints run between commits
		}
		hang()
	case "kill-mid-transaction":
		db := open(fastCheckpoints)
		insert(db, "things", "doc", 50)
		_ = db.Update(func(tx *Tx) error {
			for i := 0; i < 300; i++ {
				if _, err := tx.Collection("pending").Insert(Document{"_id": fmt.Sprintf("p-%03d", i)}); err != nil {
					return err
				}
			}
			time.Sleep(50 * time.Millisecond) // checkpoints are due while the transaction is open
			hang()                            // dies inside the open transaction
			return nil
		})
	case "rollback-then-hang":
		db := open(Background(false))
		insert(db, "things", "doc", 50)
		_ = db.Update(func(tx *Tx) error {
			_, _ = tx.Collection("ghost").Insert(Document{"_id": "g"})
			_, _ = tx.Collection("things").Insert(Document{"_id": "extra"})
			return errors.New("abort")
		})
		hang()
	case "commit-one-more":
		db := open(Background(false))
		n := 0
		_ = db.View(func(tx *Tx) error {
			docs, _ := tx.Collection("things").Query().All()
			n = len(docs)
			return nil
		})
		insert(db, "things", fmt.Sprintf("round%d", n/10), 10)
		hang()
	default:
		fail(fmt.Errorf("unknown mode %q", mode))
	}
}
