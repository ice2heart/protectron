package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
)

// stat bumps a usage counter, fire-and-forget: a stats failure must never
// break a captcha flow.
func (h *Handlers) stat(ctx context.Context, chatID int64, counter string) {
	if err := h.store.Stats.Inc(ctx, chatID, counter); err != nil {
		slog.Error("stats increment failed", "chat_id", chatID, "counter", counter, "err", err)
	}
}

// Stats answers the super admin (ADMIN_ID) in a private chat with per-chat
// usage totals, all time and last 7 days. The data spans every chat the bot
// serves, so nobody else — and no group — gets it.
func (h *Handlers) Stats(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg.Chat.Type != models.ChatTypePrivate || msg.From == nil {
		return
	}
	if h.adminID == 0 || msg.From.ID != h.adminID {
		h.reply(ctx, b, msg, h.msgs.T(i18n.FallbackLang, "admins_only_warn", nil))
		return
	}

	allTime, err := h.store.Stats.TotalsSince(ctx, "")
	if err != nil {
		slog.Error("stats query failed", "err", err)
		h.reply(ctx, b, msg, h.msgs.T(i18n.FallbackLang, "something_gone_wrong_warn", nil))
		return
	}
	weekAgo := storage.Day(time.Now().AddDate(0, 0, -7))
	lastWeek, err := h.store.Stats.TotalsSince(ctx, weekAgo)
	if err != nil {
		slog.Error("stats query failed", "err", err)
		h.reply(ctx, b, msg, h.msgs.T(i18n.FallbackLang, "something_gone_wrong_warn", nil))
		return
	}
	for _, text := range h.formatStats(ctx, allTime, lastWeek) {
		h.reply(ctx, b, msg, text)
	}
}

// statsChatsPerMessage caps how many chats go into one reply, keeping each
// message under Telegram's 4096-character limit.
const statsChatsPerMessage = 10

// formatStats renders the totals as one or more messages, statsChatsPerMessage
// chats each.
func (h *Handlers) formatStats(ctx context.Context, allTime, lastWeek []storage.ChatTotals) []string {
	if len(allTime) == 0 {
		return []string{"No stats yet."}
	}
	week := make(map[int64]storage.ChatTotals, len(lastWeek))
	for _, t := range lastWeek {
		week[t.ChatID] = t
	}

	var messages []string
	var sb strings.Builder
	sb.WriteString("Usage stats — all time (last 7 days):\n")
	for i, t := range allTime {
		if i > 0 && i%statsChatsPerMessage == 0 {
			messages = append(messages, sb.String())
			sb.Reset()
		}
		title, username := "", ""
		if settings, err := h.store.Chats.Get(ctx, t.ChatID); err == nil {
			title = settings.Title
			username = settings.Username
		}
		if title == "" {
			title = "?"
		}
		w := week[t.ChatID]
		if username != "" {
			fmt.Fprintf(&sb, "\n%s — https://t.me/%s (%d)\n", title, username, t.ChatID)
		} else {
			fmt.Fprintf(&sb, "\n%s (%d)\n", title, t.ChatID)
		}
		fmt.Fprintf(&sb, "  joins: %d (%d)\n", t.Joins, w.Joins)
		fmt.Fprintf(&sb, "  passed: %d (%d)\n", t.Passed, w.Passed)
		fmt.Fprintf(&sb, "  failed: %d (%d)\n", t.Failed, w.Failed)
		fmt.Fprintf(&sb, "  timeouts: %d (%d)\n", t.Timeouts, w.Timeouts)
		fmt.Fprintf(&sb, "  leaves: %d (%d)\n", t.Leaves, w.Leaves)
	}
	return append(messages, sb.String())
}
