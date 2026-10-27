package sweep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// errLocked means another process holds the lock.
var errLocked = errors.New("locked")

// lockRetry is how often a waiting writer retries a held lock.
const lockRetry = 50 * time.Millisecond

// lock takes an exclusive advisory lock on path, creating the file. With
// wait, it retries until the lock is free or ctx ends; without, it returns
// errLocked at once. The kernel drops the lock when its holder exits, however
// it exits, so a crash never leaves a stale lock. unlock releases it.
func lock(ctx context.Context, path string, wait bool) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if !wait {
			_ = f.Close()
			return nil, errLocked
		}
		if err := sleep(ctx, lockRetry); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
}

// lockState serialises the state file's writers (daemon sweep, idle-now,
// pane-closed hook): each holds it from read to write, so none overwrites
// what another changed in between.
func lockState(ctx context.Context, statePath string) (unlock func(), err error) {
	return lock(ctx, statePath+".lock", true)
}
