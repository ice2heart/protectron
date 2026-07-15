// Package sweeper periodically expires overdue captcha sessions.
//
// Expiry has side effects (kick, ban, message cleanup), which is why the
// sessions collection uses a plain expires_at index instead of a Mongo TTL
// index: the sweeper must act before the document goes away.
package sweeper

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"github.com/ice2heart/protectron/internal/handlers"
	"github.com/ice2heart/protectron/internal/storage"
)

const DefaultInterval = 30 * time.Second

// Run blocks until ctx is done, scanning for expired sessions every interval.
// Failures are logged per item and never stop the loop.
func Run(ctx context.Context, interval time.Duration, store *storage.Store, b *bot.Bot, h *handlers.Handlers) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep(ctx, store, b, h)
		}
	}
}

func sweep(ctx context.Context, store *storage.Store, b *bot.Bot, h *handlers.Handlers) {
	expired, err := store.Sessions.ListExpired(ctx, time.Now().UTC())
	if err != nil {
		slog.Error("sweeper: list expired failed", "err", err)
		return
	}
	for _, session := range expired {
		if ctx.Err() != nil {
			return
		}
		h.ExpireSession(ctx, b, session.ID)
	}
}
