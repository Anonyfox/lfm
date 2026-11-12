package release

import (
	"context"
	"errors"
	"testing"
)

func TestBrowserJobsBoundAndCancellation(t *testing.T) {
	ctx := WithBrowserJobs(context.Background(), 1)
	first, err := browserJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithCancel(ctx)
	cancel()
	if done, err := browserJob(blocked); !errors.Is(err, context.Canceled) {
		if done != nil {
			done()
		}
		t.Fatalf("saturated job bound: %v", err)
	}
	first()
	done, err := browserJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done()
}
