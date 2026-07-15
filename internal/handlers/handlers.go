// Package handlers contains the telegram update handlers.
package handlers

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
)

type Handlers struct {
	store   *storage.Store
	msgs    *i18n.Bundle
	adminID int64
	locks   sessionLocks
	admins  adminCache
}

func New(store *storage.Store, msgs *i18n.Bundle, adminID int64) *Handlers {
	return &Handlers{
		store:   store,
		msgs:    msgs,
		adminID: adminID,
	}
}

// lang picks the chat's configured language, guarding against values whose
// templates are gone.
func (h *Handlers) lang(settings *storage.ChatSettings) string {
	if h.msgs.Has(settings.Lang) {
		return settings.Lang
	}
	return i18n.FallbackLang
}

func (h *Handlers) Ping(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	slog.Info("ping requested", "chat_id", msg.Chat.ID, "chat_title", msg.Chat.Title)
	settings, err := h.store.Chats.Get(ctx, msg.Chat.ID)
	if err != nil {
		slog.Error("chat settings load failed", "chat_id", msg.Chat.ID, "err", err)
		settings = storage.DefaultChatSettings(msg.Chat.ID)
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   h.msgs.T(h.lang(settings), "pong_msg", nil),
		ReplyParameters: &models.ReplyParameters{
			MessageID: msg.ID,
		},
	})
	if err != nil {
		slog.Error("ping reply failed", "chat_id", msg.Chat.ID, "err", err)
	}
}
