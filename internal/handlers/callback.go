package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/captcha"
	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
)

// Callback handles every captcha button press. Validation order per
// architecture.md (cheap → expensive): shape, session, message id, user,
// token. Presses on one session are serialized by a per-session lock;
// input updates go through findOneAndUpdate on top of that.
func (h *Handlers) Callback(ctx context.Context, b *bot.Bot, update *models.Update) {
	cq := update.CallbackQuery

	sessionID, token, ok := captcha.ParseCallbackData(cq.Data)
	if !ok {
		answer(ctx, b, cq.ID, "")
		return
	}

	unlock := h.locks.lock(sessionID)
	defer unlock()

	session, err := h.store.Sessions.Get(ctx, sessionID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			slog.Error("session load failed", "session_id", sessionID, "err", err)
		}
		answer(ctx, b, cq.ID, h.msgs.T(i18nLangOrDefault(h, ctx, cq), "something_gone_wrong_warn", nil))
		return
	}

	settings, err := h.store.Chats.Get(ctx, session.ChatID)
	if err != nil {
		slog.Error("chat settings load failed", "session_id", sessionID, "err", err)
		settings = storage.DefaultChatSettings(session.ChatID)
	}
	lang := h.lang(settings)

	// Reject presses on a stale keyboard (e.g. the pre-retry captcha).
	if cq.Message.Message == nil || cq.Message.Message.ID != session.MessageIDs.Captcha {
		answer(ctx, b, cq.ID, h.msgs.T(lang, "something_gone_wrong_warn", nil))
		return
	}
	if cq.From.ID != session.UserID {
		answer(ctx, b, cq.ID, h.msgs.T(lang, "not_for_you_warn", nil))
		return
	}

	if captcha.IsBackspace(token) {
		session, err = h.store.Sessions.PopInput(ctx, sessionID)
		if err != nil {
			slog.Error("backspace failed", "session_id", sessionID, "err", err)
			answer(ctx, b, cq.ID, h.msgs.T(lang, "something_gone_wrong_warn", nil))
			return
		}
		slog.Info("backspace", "debug_id", session.DebugID)
		answer(ctx, b, cq.ID, session.InputString())
		return
	}

	char, known := session.Buttons[token]
	if !known {
		answer(ctx, b, cq.ID, "")
		return
	}

	session, err = h.store.Sessions.PushInput(ctx, sessionID, char)
	if err != nil {
		slog.Error("input push failed", "session_id", sessionID, "err", err)
		answer(ctx, b, cq.ID, h.msgs.T(lang, "something_gone_wrong_warn", nil))
		return
	}
	slog.Info("press", "debug_id", session.DebugID, "input_len", len(session.Input))

	if !session.InputComplete() {
		answer(ctx, b, cq.ID, session.InputString())
		return
	}

	switch {
	case session.InputCorrect():
		h.captchaPassed(ctx, b, cq, session, settings)
	case session.Attempt < settings.MaxAttempts:
		h.captchaRetry(ctx, b, cq, session, settings)
	default:
		h.captchaFailed(ctx, b, cq, session, settings)
	}
}

func (h *Handlers) captchaPassed(ctx context.Context, b *bot.Bot, cq *models.CallbackQuery, session *storage.Session, settings *storage.ChatSettings) {
	lang := h.lang(settings)
	slog.Info("captcha passed", "debug_id", session.DebugID)

	if err := h.store.Sessions.Delete(ctx, session.ID); err != nil {
		slog.Error("session delete failed", "session_id", session.ID, "err", err)
	}
	h.stat(ctx, session.ChatID, storage.StatPassed)
	answer(ctx, b, cq.ID, "✅")

	if err := unmute(ctx, b, session.ChatID, session.UserID); err != nil {
		slog.Error("unmute failed", "debug_id", session.DebugID, "err", err)
	}
	deleteSessionMessages(ctx, b, session)

	chatTitle := settings.Title
	if chatTitle == "" && cq.Message.Message != nil {
		chatTitle = cq.Message.Message.Chat.Title
	}
	params := map[string]string{
		"user_title": userTitle(&cq.From),
		"chat_title": chatTitle,
	}
	text := settings.Greeting
	if text == "" {
		text = h.msgs.T(lang, "success_msg", params)
	} else {
		text = i18n.Expand(text, params)
	}
	h.send(ctx, b, session.ChatID, text)
}

func (h *Handlers) captchaRetry(ctx context.Context, b *bot.Bot, cq *models.CallbackQuery, session *storage.Session, settings *storage.ChatSettings) {
	lang := h.lang(settings)
	slog.Info("captcha wrong, retrying", "debug_id", session.DebugID, "attempt", session.Attempt)

	c, messageID, err := h.sendCaptcha(ctx, b, settings, session.ID, "retry_msg", &cq.From)
	if err != nil {
		slog.Error("retry captcha send failed", "debug_id", session.DebugID, "err", err)
		answer(ctx, b, cq.ID, h.msgs.T(lang, "something_gone_wrong_warn", nil))
		return
	}

	oldCaptchaMessage := session.MessageIDs.Captcha
	expiresAt := time.Now().UTC().Add(settings.CaptchaTimeout())
	if _, err := h.store.Sessions.NewAttempt(ctx, session.ID, c.Answer, c.Buttons, messageID, expiresAt); err != nil {
		slog.Error("attempt swap failed", "debug_id", session.DebugID, "err", err)
		deleteMessages(ctx, b, session.ChatID, messageID)
		answer(ctx, b, cq.ID, h.msgs.T(lang, "something_gone_wrong_warn", nil))
		return
	}
	deleteMessages(ctx, b, session.ChatID, oldCaptchaMessage)
	answer(ctx, b, cq.ID, h.msgs.T(lang, "fail_msg", nil))
}

func (h *Handlers) captchaFailed(ctx context.Context, b *bot.Bot, cq *models.CallbackQuery, session *storage.Session, settings *storage.ChatSettings) {
	lang := h.lang(settings)
	until := time.Now().Add(settings.BanDuration())
	slog.Info("captcha failed, banning", "debug_id", session.DebugID, "until", until)

	if err := h.store.Sessions.Delete(ctx, session.ID); err != nil {
		slog.Error("session delete failed", "session_id", session.ID, "err", err)
	}
	h.stat(ctx, session.ChatID, storage.StatFailed)
	answer(ctx, b, cq.ID, h.msgs.T(lang, "fail_msg", nil))

	if _, err := b.BanChatMember(ctx, &bot.BanChatMemberParams{
		ChatID:    session.ChatID,
		UserID:    session.UserID,
		UntilDate: int(until.Unix()),
	}); err != nil {
		slog.Error("ban failed", "debug_id", session.DebugID, "err", err)
	}
	deleteSessionMessages(ctx, b, session)
}

// i18nLangOrDefault picks a language when there is no session to tell us the
// chat: use the callback's chat when accessible, else the fallback.
func i18nLangOrDefault(h *Handlers, ctx context.Context, cq *models.CallbackQuery) string {
	if cq.Message.Message != nil {
		if settings, err := h.store.Chats.Get(ctx, cq.Message.Message.Chat.ID); err == nil {
			return h.lang(settings)
		}
	}
	return i18n.FallbackLang
}
