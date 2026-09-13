package github

import "context"

type retryObserverKey struct{}

// RetryObserver is called before each 202 "still computing" retry with the
// request path and the attempt number about to be made (1-based).
type RetryObserver func(path string, attempt int)

// WithRetryObserver returns a context whose requests report 202 retries to fn.
func WithRetryObserver(ctx context.Context, fn RetryObserver) context.Context {
	return context.WithValue(ctx, retryObserverKey{}, fn)
}

func retryObserverFrom(ctx context.Context) RetryObserver {
	fn, _ := ctx.Value(retryObserverKey{}).(RetryObserver)
	return fn
}
