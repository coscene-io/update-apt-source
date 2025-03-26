package locker

import (
	"fmt"
	"time"

	"github.com/coscene-io/update-apt-source/storage"
)

const (
	lockFilePath      = "apt-repo.lock"
	maxLockWait       = 60 * time.Second
	lockCheckInterval = 10 * time.Second
)

type Locker struct {
	storage    storage.StorageProvider
	bucketName string
}

func NewLocker(storage storage.StorageProvider, bucketName string) *Locker {
	fmt.Println("\nInitializing lock manager... ✓")
	return &Locker{
		storage:    storage,
		bucketName: bucketName,
	}
}

func (l *Locker) Lock() error {
	lockContent := fmt.Sprintf("Locked by process at %s", time.Now().Format(time.RFC3339))

	// Check if the lock file exists
	exists, err := l.storage.HeadObject(l.bucketName, lockFilePath)
	if err != nil {
		return fmt.Errorf("❌ Check lock file failed: %v", err)
	}

	if exists {
		fmt.Println("  ⏳ Lock file exists, waiting for release...")
		deadline := time.Now().Add(maxLockWait)

		for time.Now().Before(deadline) {
			time.Sleep(lockCheckInterval)

			exists, err = l.storage.HeadObject(l.bucketName, lockFilePath)
			if err != nil {
				return fmt.Errorf("❌ Check lock file failed: %v", err)
			}

			if !exists {
				break
			}

			fmt.Println("  ⏳ Lock file still exists, continue waiting...")
		}
		if exists {
			return fmt.Errorf("❌ Wait for lock release timeout (%v)", maxLockWait)
		}
	}

	err = l.storage.PutObject(l.bucketName, lockFilePath, []byte(lockContent))
	if err != nil {
		return fmt.Errorf("❌ Create lock file failed: %v", err)
	}

	fmt.Println("\n🔒 APT source repository locked!")
	return nil
}

func (l *Locker) Unlock() error {
	// Check if the lock file exists
	exists, err := l.storage.HeadObject(l.bucketName, lockFilePath)
	if err != nil {
		return fmt.Errorf("❌ Check lock file failed: %v", err)
	}

	if !exists {
		// The lock file does not exist, no action needed
		return nil
	}

	err = l.storage.DeleteObject(l.bucketName, lockFilePath)
	if err != nil {
		return fmt.Errorf("❌ Delete lock file failed: %v", err)
	}

	fmt.Println("\n🔓 APT source repository unlocked!")
	return nil
}
