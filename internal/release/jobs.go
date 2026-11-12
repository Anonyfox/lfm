package release

import "context"

type jobsKey struct{}

// WithBrowserJobs gives all browser launches in one compile a shared bound.
// Viewports, frames and printer batches are sequential within each launch.
func WithBrowserJobs(ctx context.Context, n int) context.Context {
	if n < 1 {
		n = 2
	}
	return context.WithValue(ctx, jobsKey{}, make(chan struct{}, n))
}

func browserJob(ctx context.Context) (func(), error) {
	jobs, _ := ctx.Value(jobsKey{}).(chan struct{})
	if jobs == nil {
		return func() {}, nil
	}
	select {
	case jobs <- struct{}{}:
		return func() { <-jobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
