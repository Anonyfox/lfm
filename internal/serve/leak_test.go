package serve

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// TestStoppingLeavesNothingRunning stops a preview while a browser holds
// its reload stream open: every goroutine it started ends with it.
func TestStoppingLeavesNothingRunning(t *testing.T) {
	root := testProject(t)
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	addresses := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, root, Options{Port: 0, ExplicitPort: true, Log: logURL{addresses}}) }()
	address := <-addresses
	res, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	src := regexp.MustCompile(`src="(/__lfm_[0-9a-f]+)/client\.js`).FindSubmatch(page)
	if src == nil {
		t.Fatalf("no reload client in %s", page)
	}
	events, err := http.Get(address + string(src[1])[1:] + "/events")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(events.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the preview did not stop")
	}
	events.Body.Close()
	http.DefaultClient.CloseIdleConnections()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Fatalf("%d goroutines before, %d after:\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}
