package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
)

const adminCacheTTL = 5 * time.Minute

type adminCacheKey struct {
	chatID int64
	userID int64
}

type adminCacheEntry struct {
	isAdmin bool
	expires time.Time
}

type adminCache struct {
	mu      sync.Mutex
	entries map[adminCacheKey]adminCacheEntry
}

func (c *adminCache) get(chatID, userID int64) (isAdmin, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[adminCacheKey{chatID, userID}]
	if !ok || time.Now().After(e.expires) {
		return false, false
	}
	return e.isAdmin, true
}

func (c *adminCache) put(chatID, userID int64, isAdmin bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[adminCacheKey]adminCacheEntry)
	}
	c.entries[adminCacheKey{chatID, userID}] = adminCacheEntry{
		isAdmin: isAdmin,
		expires: time.Now().Add(adminCacheTTL),
	}
}

func (h *Handlers) isChatAdmin(ctx context.Context, b *bot.Bot, chatID, userID int64) bool {
	if isAdmin, found := h.admins.get(chatID, userID); found {
		return isAdmin
	}
	m, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		slog.Error("admin check failed", "chat_id", chatID, "user_id", userID, "err", err)
		return false
	}
	isAdmin := m.Type == models.ChatMemberTypeOwner || m.Type == models.ChatMemberTypeAdministrator
	h.admins.put(chatID, userID, isAdmin)
	return isAdmin
}

func isGroup(chat models.Chat) bool {
	return chat.Type == models.ChatTypeGroup || chat.Type == models.ChatTypeSupergroup
}

// adminCommandPrologue does the shared checks; ok=false means already replied
// or the command should be ignored.
func (h *Handlers) adminCommandPrologue(ctx context.Context, b *bot.Bot, msg *models.Message) (settings *storage.ChatSettings, lang string, ok bool) {
	if !isGroup(msg.Chat) || msg.From == nil {
		return nil, "", false
	}
	settings, err := h.store.Chats.Get(ctx, msg.Chat.ID)
	if err != nil {
		slog.Error("chat settings load failed", "chat_id", msg.Chat.ID, "err", err)
		settings = storage.DefaultChatSettings(msg.Chat.ID)
	}
	lang = h.lang(settings)
	if !h.isChatAdmin(ctx, b, msg.Chat.ID, msg.From.ID) {
		h.reply(ctx, b, msg, h.msgs.T(lang, "admins_only_warn", nil))
		return nil, "", false
	}
	return settings, lang, true
}

// Settings shows the current per-chat configuration.
func (h *Handlers) Settings(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	settings, lang, ok := h.adminCommandPrologue(ctx, b, msg)
	if !ok {
		return
	}
	greeting := settings.Greeting
	if greeting == "" {
		greeting = "-"
	}
	h.reply(ctx, b, msg, h.msgs.T(lang, "settings_msg", map[string]string{
		"lang":     settings.Lang,
		"timeout":  strconv.Itoa(settings.CaptchaTimeoutSec),
		"length":   strconv.Itoa(settings.CaptchaLength),
		"attempts": strconv.Itoa(settings.MaxAttempts),
		"ban":      strconv.Itoa(settings.BanDurationSec),
		"greeting": greeting,
	}))
}

// setKeys maps /set keys to the Mongo field and the accepted integer range.
var setKeys = map[string]struct {
	field    string
	min, max int
}{
	"timeout":  {"captcha_timeout_sec", 60, 3600},
	"length":   {"captcha_length", 4, 10},
	"attempts": {"max_attempts", 1, 5},
	"ban":      {"ban_duration_sec", 30, 86400},
}

// Set changes one setting: /set lang ru|en, /set timeout 60..3600, …
func (h *Handlers) Set(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	_, lang, ok := h.adminCommandPrologue(ctx, b, msg)
	if !ok {
		return
	}
	badValue := func() {
		h.reply(ctx, b, msg, h.msgs.T(lang, "set_bad_value_msg", nil))
	}

	parts := strings.SplitN(msg.Text, " ", 3)
	if len(parts) != 3 {
		badValue()
		return
	}
	key, value := parts[1], parts[2]

	var field string
	var stored any
	switch {
	case key == "lang":
		if !h.msgs.Has(value) {
			badValue()
			return
		}
		field, stored = "lang", value
	case key == "greeting":
		if value == "-" {
			value = ""
		}
		field, stored = "greeting", value
	default:
		spec, known := setKeys[key]
		if !known {
			badValue()
			return
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < spec.min || n > spec.max {
			badValue()
			return
		}
		field, stored = spec.field, n
	}

	if err := h.store.Chats.SetField(ctx, msg.Chat.ID, field, stored); err != nil {
		slog.Error("set field failed", "chat_id", msg.Chat.ID, "field", field, "err", err)
		h.reply(ctx, b, msg, h.msgs.T(lang, "something_gone_wrong_warn", nil))
		return
	}
	slog.Info("setting changed", "chat_id", msg.Chat.ID, "field", field, "value", stored, "by", msg.From.ID)
	h.reply(ctx, b, msg, h.msgs.T(lang, "set_ok_msg", nil))
}

// renderGreeting builds the post-captcha welcome for user in chatTitle: the
// admin-supplied greeting when set, otherwise the localised default. The
// result is Markdown and carries a clickable mention.
func (h *Handlers) renderGreeting(settings *storage.ChatSettings, lang string, user *models.User, chatTitle string) string {
	params := map[string]string{
		"user_title": userMention(user),
		"chat_title": mdEscaper.Replace(chatTitle),
	}
	if settings.Greeting == "" {
		return h.msgs.T(lang, "success_msg", params)
	}
	// Custom greetings are plain text: escape the literal spans only, so the
	// mention link is the only markup in the message. Escaping the whole
	// string first would break the ${var} syntax itself.
	return i18n.ExpandFunc(settings.Greeting, params, func(s string) string {
		return mdEscaper.Replace(unescapeNewlines(s))
	})
}

// Test previews the greeting exactly as a joining member would see it.
func (h *Handlers) Test(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	settings, lang, ok := h.adminCommandPrologue(ctx, b, msg)
	if !ok {
		return
	}
	chatTitle := settings.Title
	if chatTitle == "" {
		chatTitle = msg.Chat.Title
	}
	h.sendMarkdown(ctx, b, msg.Chat.ID, h.renderGreeting(settings, lang, msg.From, chatTitle))
}

// reply answers in-thread, logging failures.
func (h *Handlers) reply(ctx context.Context, b *bot.Bot, msg *models.Message, text string) {
	_, err := tgCall(ctx, "sendMessage(reply)", func(ctx context.Context) (*models.Message, error) {
		return b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		})
	})
	if err != nil {
		slog.Error("reply failed", "chat_id", msg.Chat.ID, "err", err)
	}
}
