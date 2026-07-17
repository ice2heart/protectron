package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
)

const (
	tgRetryAttempts  = 3
	tgRetryBaseDelay = time.Second
	tgRetryMaxDelay  = 30 * time.Second
)

// permanentTGErrors are telegram responses that won't change on retry.
var permanentTGErrors = []error{
	bot.ErrorBadRequest,
	bot.ErrorForbidden,
	bot.ErrorUnauthorized,
	bot.ErrorNotFound,
	bot.ErrorConflict,
}

// tgCall runs one telegram API call, retrying transient failures (network
// timeouts, 5xx) with backoff; a 429 waits the server-provided retry_after.
// Permanent bad-request-class errors are returned immediately.
func tgCall[T any](ctx context.Context, op string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		v, err := fn(ctx)
		if err == nil {
			return v, nil
		}
		if attempt == tgRetryAttempts || !retryableTG(err) {
			return zero, err
		}
		delay := tgRetryBaseDelay << (attempt - 1)
		var tooMany *bot.TooManyRequestsError
		if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
			delay = time.Duration(tooMany.RetryAfter) * time.Second
		}
		if delay > tgRetryMaxDelay {
			delay = tgRetryMaxDelay
		}
		slog.Warn("telegram call failed, retrying", "op", op, "attempt", attempt, "delay", delay, "err", err)
		select {
		case <-ctx.Done():
			return zero, err
		case <-time.After(delay):
		}
	}
}

func retryableTG(err error) bool {
	var tooMany *bot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		return true
	}
	for _, sentinel := range permanentTGErrors {
		if errors.Is(err, sentinel) {
			return false
		}
	}
	return true
}
