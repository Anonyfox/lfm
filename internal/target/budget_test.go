package target

import (
	"io"
	"strings"
	"sync"
	"testing"
)

// waitObserver signals when Cond.Wait releases its lock, so the test can
// return an in-flight read's unused allowance after another reader waits.
type waitObserver struct {
	sync.Locker
	waiting chan struct{}
}

func (l waitObserver) Unlock() {
	l.Locker.Unlock()
	close(l.waiting)
}

func TestBudgetWaitsForUnusedBytes(t *testing.T) {
	t.Parallel()
	// An in-flight read has claimed the last byte but has not consumed it.
	budget := &byteBudget{pending: 1}
	waiting := make(chan struct{})
	budget.ready = sync.NewCond(waitObserver{&budget.mu, waiting})
	reader := &budgetReader{Reader: strings.NewReader("x"), budget: budget}
	done := make(chan struct{})
	var n int
	var err error
	data := make([]byte, 1)
	go func() {
		defer close(done)
		n, err = reader.Read(data)
	}()
	<-waiting
	budget.mu.Lock()
	budget.pending = 0
	budget.remaining = 1
	budget.ready.Broadcast()
	budget.mu.Unlock()
	<-done
	if n != 1 || err != nil || string(data) != "x" || reader.exhausted {
		t.Fatalf("unused allowance was lost: n=%d err=%v data=%q exhausted=%v", n, err, data, reader.exhausted)
	}
	if n, err := reader.Read(data); n != 0 || err != io.EOF || !reader.exhausted {
		t.Fatalf("consumed budget: n=%d err=%v exhausted=%v", n, err, reader.exhausted)
	}
}
