package locker

import (
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

const (
	lockFilePath      = "coscene-apt-source/apt-repo.lock"
	maxLockWait       = 60 * time.Minute
	lockCheckInterval = 10 * time.Second
)

type Locker struct {
	bucket *oss.Bucket
}

func NewLocker(bucket *oss.Bucket) *Locker {
	return &Locker{bucket: bucket}
}

func (l *Locker) Lock() error {
	lockContent := fmt.Sprintf("Locked by process at %s", time.Now().Format(time.RFC3339))
	exist, err := l.bucket.IsObjectExist(lockFilePath)
	if err != nil {
		return fmt.Errorf("check lock file failed: %v", err)
	}

	if exist {
		fmt.Println("lock file found, waiting for release...")
		deadline := time.Now().Add(maxLockWait)

		for time.Now().Before(deadline) {
			time.Sleep(lockCheckInterval)

			exist, err := l.bucket.IsObjectExist(lockFilePath)
			if err != nil {
				return fmt.Errorf("check lock file failed: %v", err)
			}

			if !exist {
				break
			}

			fmt.Println("lock file still exists, waiting...")
		}

		exist, err = l.bucket.IsObjectExist(lockFilePath)
		if err != nil {
			return fmt.Errorf("check lock file failed: %v", err)
		}

		if exist {
			return fmt.Errorf("timeout waiting for lock (%v)", maxLockWait)
		}
	}

	err = l.bucket.PutObject(lockFilePath, strings.NewReader(lockContent))
	if err != nil {
		return fmt.Errorf("create lock file failed: %v", err)
	}

	fmt.Println("lock file created")
	return nil
}

func (l *Locker) Unlock() error {
	exist, err := l.bucket.IsObjectExist(lockFilePath)
	if err != nil {
		return fmt.Errorf("check lock file failed: %v", err)
	}

	if exist {
		err = l.bucket.DeleteObject(lockFilePath)
		if err != nil {
			return fmt.Errorf("delete lock file failed: %v", err)
		}
		fmt.Println("lock file deleted")
	}

	return nil
}
