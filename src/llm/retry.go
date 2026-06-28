package llm

import (
	stdctx "context"
	"errors"
	"time"
)

const (
	defaultLLMMaxRetries = 1
	maxLLMMaxRetries     = 2
	defaultRetryBase     = 25 * time.Millisecond
	defaultRetryMaxDelay = 100 * time.Millisecond
)

func withLLMRetry(ctx stdctx.Context, cfg Config, req Request, operation func() (*Response, error)) (*Response, error) {
	policy := resolveRetryPolicy(cfg, req)
	for attempt := 0; ; attempt++ {
		resp, err := operation()
		if err == nil {
			return resp, nil
		}
		if policy.Disabled || attempt >= policy.MaxRetries || !shouldRetryLLMError(err) {
			return nil, err
		}
		if waitErr := sleepForRetry(ctx, retryDelay(policy, attempt)); waitErr != nil {
			return nil, waitErr
		}
	}
}

func resolveRetryPolicy(cfg Config, req Request) RetryPolicy {
	policy := cfg.Retry
	if req.Retry != nil {
		policy = *req.Retry
	}
	if policy.Disabled {
		return policy
	}
	if policy.MaxRetries == 0 {
		policy.MaxRetries = defaultLLMMaxRetries
	}
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.MaxRetries > maxLLMMaxRetries {
		policy.MaxRetries = maxLLMMaxRetries
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = defaultRetryBase
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = defaultRetryMaxDelay
	}
	if policy.MaxDelay < policy.BaseDelay {
		policy.MaxDelay = policy.BaseDelay
	}
	return policy
}

func shouldRetryLLMError(err error) bool {
	var requestErr *RequestError
	if !errors.As(err, &requestErr) {
		return false
	}
	return requestErr.Retryable()
}

func isRetryableErrorKind(kind ErrorKind) bool {
	return kind == ErrorKindRateLimit ||
		kind == ErrorKindServerError ||
		kind == ErrorKindServerOverload ||
		kind == ErrorKindTransport
}

func retryDelay(policy RetryPolicy, attempt int) time.Duration {
	delay := policy.BaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= policy.MaxDelay {
			return policy.MaxDelay
		}
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func sleepForRetry(ctx stdctx.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
