package file

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLockIsExclusiveAndReusable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "x.coco")
	l1, err := AcquireLock(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(db); !errors.Is(err, ErrDatabaseLocked) {
		t.Fatalf("second lock in the same process: %v, want ErrDatabaseLocked", err)
	}
	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := AcquireLock(db)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = l2.Release()
}

// A lock file left behind (by an older CoCoDB, or by hand) must not block anyone.
func TestLeftoverLockFileIsIgnored(t *testing.T) {
	db := filepath.Join(t.TempDir(), "x.coco")
	if err := os.WriteFile(db+"-lock", []byte("12345\n"), 0600); err != nil {
		t.Fatal(err)
	}
	l, err := AcquireLock(db)
	if err != nil {
		t.Fatalf("lock with a leftover lock file: %v", err)
	}
	_ = l.Release()
}

// The lock must die with the process that holds it, however that process ends.
func TestLockIsReleasedWhenTheHolderDies(t *testing.T) {
	db := filepath.Join(t.TempDir(), "x.coco")
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHoldLock$")
	cmd.Env = append(os.Environ(), "COCO_LOCK_HELPER="+db)
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
			if s.Text() == "locked" {
				ready <- "locked"
				return
			}
		}
		ready <- "helper exited without locking"
	}()
	select {
	case msg := <-ready:
		if msg != "locked" {
			t.Fatal(msg)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper did not take the lock")
	}

	if _, err := AcquireLock(db); !errors.Is(err, ErrDatabaseLocked) {
		t.Fatalf("lock while another process holds it: %v, want ErrDatabaseLocked", err)
	}

	// Kill the holder without letting it release anything.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	if _, err := os.Stat(db + "-lock"); err != nil {
		t.Fatalf("expected the dead holder's lock file to remain: %v", err)
	}
	l, err := AcquireLock(db)
	if err != nil {
		t.Fatalf("lock after the holder was killed: %v", err)
	}
	_ = l.Release()
}

// TestHelperHoldLock runs only as the child process of TestLockIsReleasedWhenTheHolderDies.
func TestHelperHoldLock(t *testing.T) {
	db := os.Getenv("COCO_LOCK_HELPER")
	if db == "" {
		t.Skip("helper process only")
	}
	if _, err := AcquireLock(db); err != nil {
		os.Stdout.WriteString("error: " + err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("locked\n")
	time.Sleep(time.Minute) // killed by the parent long before this
	os.Exit(3)
}
