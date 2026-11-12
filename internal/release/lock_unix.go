//go:build unix

package release

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

var browserLock struct {
	sync.Mutex
	file      *os.File
	refs      int
	acquiring chan struct{}
}

// AcquireBrowser holds the box-wide lock until the returned release is called.
// References within this process share ownership, including nested printers.
func AcquireBrowser(ctx context.Context) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		browserLock.Lock()
		if browserLock.refs > 0 {
			browserLock.refs++
			browserLock.Unlock()
			return sync.OnceFunc(releaseBrowser), nil
		}
		if pending := browserLock.acquiring; pending != nil {
			browserLock.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending:
				continue
			}
		}
		browserLock.acquiring = make(chan struct{})
		browserLock.Unlock()
		f, err := lockBrowser(ctx)
		browserLock.Lock()
		if err == nil {
			browserLock.file, browserLock.refs = f, 1
		}
		close(browserLock.acquiring)
		browserLock.acquiring = nil
		browserLock.Unlock()
		if err != nil {
			return nil, err
		}
		return sync.OnceFunc(releaseBrowser), nil
	}
}

func releaseBrowser() {
	browserLock.Lock()
	defer browserLock.Unlock()
	browserLock.refs--
	if browserLock.refs == 0 {
		_ = syscall.Flock(int(browserLock.file.Fd()), syscall.LOCK_UN)
		_ = browserLock.file.Close()
		browserLock.file = nil
	}
}

func lockBrowser(ctx context.Context) (*os.File, error) {
	path := os.Getenv("LFM_BROWSER_LOCK")
	if path == "" {
		path = "/tmp/lfm-browser.lock"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	_ = f.Chmod(0666)
	waiting := false
	for {
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			return nil, err
		}
		if !waiting {
			raw, _ := os.ReadFile(path)
			fmt.Fprintf(os.Stderr, "lfm: waiting for the box-wide browser lock (held by PID %s, %s)\n", string(raw), path)
			waiting = true
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d", os.Getpid())
	return f, nil
}
