package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/captcha"
	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
)

// MatchChatMember routes chat_member updates (no typed registration exists
// in the framework for them).
func MatchChatMember(update *models.Update) bool {
	return update.ChatMember != nil
}

// MatchNewChatMembers routes the join service messages.
func MatchNewChatMembers(update *models.Update) bool {
	return update.Message != nil && len(update.Message.NewChatMembers) > 0
}

// MatchLeftChatMember routes the leave service messages.
func MatchLeftChatMember(update *models.Update) bool {
	return update.Message != nil && update.Message.LeftChatMember != nil
}

// ChatMemberUpdate is the join/leave trigger (see architecture.md: the
// service messages are tracked only for cleanup).
func (h *Handlers) ChatMemberUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	upd := update.ChatMember
	user := memberUser(upd.NewChatMember)
	if user == nil {
		return
	}
	if user.ID == b.ID() {
		if !isMember(upd.OldChatMember) && isMember(upd.NewChatMember) {
			h.send(ctx, b, upd.Chat.ID, h.msgs.T(i18n.FallbackLang, "bot_added_help_msg", nil))
		}
		return
	}

	wasMember := isMember(upd.OldChatMember)
	nowMember := isMember(upd.NewChatMember)

	switch {
	case !wasMember && nowMember:
		h.handleJoin(ctx, b, upd, user)
	case wasMember && !nowMember:
		// Left or was removed mid-captcha: drop the session and its messages.
		h.cancelSession(ctx, b, upd.Chat.ID, user.ID, "left chat")
	case upd.OldChatMember.Type == models.ChatMemberTypeRestricted &&
		upd.NewChatMember.Type != models.ChatMemberTypeRestricted &&
		upd.From.ID != b.ID():
		// An admin manually lifted the restriction: the captcha is moot.
		h.cancelSession(ctx, b, upd.Chat.ID, user.ID, "manually unrestricted")
	}
}

func (h *Handlers) handleJoin(ctx context.Context, b *bot.Bot, upd *models.ChatMemberUpdated, user *models.User) {
	chatID := upd.Chat.ID
	debugID := fmt.Sprintf("%s-(%s)", upd.Chat.Title, userTitle(user))

	settings, err := h.store.Chats.Ensure(ctx, chatID, upd.Chat.Title, upd.Chat.Username)
	if err != nil {
		slog.Error("chat settings ensure failed, using defaults", "debug_id", debugID, "err", err)
		settings = storage.DefaultChatSettings(chatID)
	}
	lang := h.lang(settings)

	if user.IsBot {
		h.send(ctx, b, chatID, h.msgs.T(lang, "join_bot_msg", nil))
		return
	}
	if h.adminID != 0 && user.ID == h.adminID {
		h.send(ctx, b, chatID, h.msgs.T(lang, "join_owner_msg", nil))
		return
	}

	slog.Info("captcha started", "debug_id", debugID, "chat_id", chatID, "user_id", user.ID)
	if err := mute(ctx, b, chatID, user.ID); err != nil {
		switch {
		case notEnoughRights(err):
			slog.Warn("no restrict permission, leaving chat", "debug_id", debugID, "err", err)
			h.send(ctx, b, chatID, h.msgs.T(lang, "required_admin_permission", nil))
			if _, err := b.LeaveChat(ctx, &bot.LeaveChatParams{ChatID: chatID}); err != nil {
				slog.Error("leave chat failed", "debug_id", debugID, "err", err)
			}
		case userGone(err):
			// Joined and left again before we got to them; nothing to captcha.
			slog.Info("captcha skipped", "debug_id", debugID, "reason", "user already left")
		default:
			slog.Error("mute failed", "debug_id", debugID, "err", err)
		}
		return
	}

	// A re-join while an old captcha is still open: replace it entirely.
	if old, err := h.store.Sessions.DeleteByChatUser(ctx, chatID, user.ID); err == nil {
		deleteSessionMessages(ctx, b, old)
	}

	sessionID, err := captcha.NewSessionID()
	if err != nil {
		slog.Error("session id generation failed", "debug_id", debugID, "err", err)
		return
	}
	c, messageID, err := h.sendCaptcha(ctx, b, settings, sessionID, "join_msg", user)
	if err != nil {
		slog.Error("send captcha failed", "debug_id", debugID, "err", err)
		return
	}

	now := time.Now().UTC()
	session := &storage.Session{
		ID:      sessionID,
		ChatID:  chatID,
		UserID:  user.ID,
		Answer:  c.Answer,
		Input:   []string{},
		Buttons: c.Buttons,
		Attempt: 1,
		MessageIDs: storage.MessageIDs{
			Captcha: messageID,
		},
		ExpiresAt: now.Add(settings.CaptchaTimeout()),
		CreatedAt: now,
		DebugID:   debugID,
	}
	if err := h.store.Sessions.Insert(ctx, session); err != nil {
		slog.Error("session insert failed", "debug_id", debugID, "err", err)
		deleteMessages(ctx, b, chatID, messageID)
		if err := unmute(ctx, b, chatID, user.ID); err != nil {
			slog.Error("rollback unmute failed", "debug_id", debugID, "err", err)
		}
		return
	}
	h.stat(ctx, chatID, storage.StatJoins)
}

// sendCaptcha generates a fresh challenge and posts it; shared by the join
// flow and the retry branch. captionKey is join_msg or retry_msg.
func (h *Handlers) sendCaptcha(ctx context.Context, b *bot.Bot, settings *storage.ChatSettings, sessionID, captionKey string, user *models.User) (*captcha.Captcha, int, error) {
	lang := h.lang(settings)
	c, err := captcha.Generate(lang, settings.CaptchaLength)
	if err != nil {
		return nil, 0, err
	}
	msg, err := tgCall(ctx, "sendPhoto(captcha)", func(ctx context.Context) (*models.Message, error) {
		// The reader is rebuilt per attempt: a failed try may have consumed it.
		return b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: settings.ID,
			Photo: &models.InputFileUpload{
				Filename: "captcha.png",
				Data:     bytes.NewReader(c.Image),
			},
			Caption:     h.msgs.T(lang, captionKey, map[string]string{"user_title": userTitle(user)}),
			ReplyMarkup: c.Keyboard(sessionID),
		})
	})
	if err != nil {
		return nil, 0, err
	}
	return c, msg.ID, nil
}

// NewChatMembers only records the join service message id for cleanup; the
// captcha itself is triggered by the chat_member update, which can arrive
// in either order relative to this message.
func (h *Handlers) NewChatMembers(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	for _, user := range msg.NewChatMembers {
		err := h.store.Sessions.SetJoinMessageID(ctx, msg.Chat.ID, user.ID, msg.ID)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			slog.Error("record join message failed", "chat_id", msg.Chat.ID, "user_id", user.ID, "err", err)
		}
	}
}

// LeftChatMember cleans up when the leave service message arrives while a
// captcha is still active, deleting that service message too (the
// chat_member path can't: it has no message id).
func (h *Handlers) LeftChatMember(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	session, err := h.store.Sessions.DeleteByChatUser(ctx, msg.Chat.ID, msg.LeftChatMember.ID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			slog.Error("leave cleanup failed", "chat_id", msg.Chat.ID, "user_id", msg.LeftChatMember.ID, "err", err)
		}
		return
	}
	slog.Info("captcha cancelled", "debug_id", session.DebugID, "reason", "left chat")
	h.stat(ctx, msg.Chat.ID, storage.StatLeaves)
	deleteSessionMessages(ctx, b, session)
	deleteMessages(ctx, b, msg.Chat.ID, msg.ID)
}

// cancelSession drops a session and its messages, if one exists.
func (h *Handlers) cancelSession(ctx context.Context, b *bot.Bot, chatID, userID int64, reason string) {
	session, err := h.store.Sessions.DeleteByChatUser(ctx, chatID, userID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			slog.Error("cancel session failed", "chat_id", chatID, "user_id", userID, "err", err)
		}
		return
	}
	slog.Info("captcha cancelled", "debug_id", session.DebugID, "reason", reason)
	h.stat(ctx, chatID, storage.StatLeaves)
	deleteSessionMessages(ctx, b, session)
}

// send posts a plain message, logging failures.
func (h *Handlers) send(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	_, err := tgCall(ctx, "sendMessage", func(ctx context.Context) (*models.Message, error) {
		return b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	})
	if err != nil {
		slog.Error("send message failed", "chat_id", chatID, "err", err)
	}
}
