package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"

	"github.com/ice2heart/protectron/internal/storage"
)

// ExpireSession applies the timeout policy to one session: kick + per-chat
// ban duration, message cleanup, session removal. Called by the sweeper; it
// takes the same per-session lock as the callback path so a last-moment
// button press and an expiry can't both resolve the session.
func (h *Handlers) ExpireSession(ctx context.Context, b *bot.Bot, sessionID string) {
	unlock := h.locks.lock(sessionID)
	defer unlock()

	// Re-load under the lock: a racing callback may have resolved it.
	session, err := h.store.Sessions.Get(ctx, sessionID)
	if err != nil {
		return
	}
	if time.Now().UTC().Before(session.ExpiresAt) {
		// A retry reset the expiry after the sweeper picked it up.
		return
	}

	settings, err := h.store.Chats.Get(ctx, session.ChatID)
	if err != nil {
		slog.Error("chat settings load failed", "session_id", sessionID, "err", err)
		settings = storage.DefaultChatSettings(session.ChatID)
	}
	until := time.Now().Add(settings.BanDuration())
	slog.Info("captcha timeout, kick and clean", "debug_id", session.DebugID, "until", until)

	if err := h.store.Sessions.Delete(ctx, session.ID); err != nil {
		slog.Error("session delete failed", "session_id", session.ID, "err", err)
		return
	}
	h.stat(ctx, session.ChatID, storage.StatTimeouts)
	if _, err := b.BanChatMember(ctx, &bot.BanChatMemberParams{
		ChatID:    session.ChatID,
		UserID:    session.UserID,
		UntilDate: int(until.Unix()),
	}); err != nil {
		slog.Error("timeout ban failed", "debug_id", session.DebugID, "err", err)
	}
	deleteSessionMessages(ctx, b, session)
}
