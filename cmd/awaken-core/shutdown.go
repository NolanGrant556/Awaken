package main

import (
	"context"
	"errors"
)

// Every owner gets the same absolute deadline. Buffered results allow a late
// uncooperative owner to finish without blocking the host's timeout path.
func joinCoreCleanup(ctx context.Context, owners ...func(context.Context) error) error {
	results := make(chan error, len(owners))
	for _, owner := range owners {
		go func(owner func(context.Context) error) { results <- owner(ctx) }(owner)
	}
	var result error
	for range owners {
		select {
		case err := <-results:
			result = errors.Join(result, err)
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		}
	}
	return result
}
