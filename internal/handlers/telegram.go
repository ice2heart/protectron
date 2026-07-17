package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/ice2heart/protectron/internal/storage"
)

// mutedPermissions revokes everything a restricted member could do.
var mutedPermissions = &models.ChatPermissions{}

// unmutedPermissions restores the messaging permissions (the same set the
// Python bot granted on success).
var unmutedPermissions = &models.ChatPermissions{
	CanSendMessages:       true,
	CanSendAudios:         true,
	CanSendDocuments:      true,
	CanSendPhotos:         true,
	CanSendVideos:         true,
	CanSendVideoNotes:     true,
	CanSendVoiceNotes:     true,
	CanSendPolls:          true,
	CanSendOtherMessages:  true,
	CanAddWebPagePreviews: true,
}

func mute(ctx context.Context, b *bot.Bot, chatID, userID int64) error {
	_, err := tgCall(ctx, "restrictChatMember(mute)", func(ctx context.Context) (bool, error) {
		return b.RestrictChatMember(ctx, &bot.RestrictChatMemberParams{
			ChatID:      chatID,
			UserID:      userID,
			Permissions: mutedPermissions,
		})
	})
	return err
}

func unmute(ctx context.Context, b *bot.Bot, chatID, userID int64) error {
	_, err := tgCall(ctx, "restrictChatMember(unmute)", func(ctx context.Context) (bool, error) {
		return b.RestrictChatMember(ctx, &bot.RestrictChatMemberParams{
			ChatID:      chatID,
			UserID:      userID,
			Permissions: unmutedPermissions,
		})
	})
	return err
}

// notEnoughRights matches telegram's "not enough rights to restrict/unrestrict
// chat member" bad request.
func notEnoughRights(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not enough rights")
}

// userGone matches the bad requests telegram returns when restricting a user
// who already left the chat ("bots can't add new chat members": restricting a
// non-member would amount to re-adding them).
func userGone(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't add new chat members") ||
		strings.Contains(msg, "user_not_participant") ||
		strings.Contains(msg, "user not found") ||
		strings.Contains(msg, "participant_id_invalid")
}

// userTitle renders a user the way the old bot's `mention` did, in plain text.
func userTitle(u *models.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	return name
}

// mdEscaper escapes the characters that are special in legacy Markdown
// (the only ones the Bot API allows escaping in that mode).
var mdEscaper = strings.NewReplacer("_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[")

// userMention renders a clickable inline mention (tg://user?id=…) for
// messages sent with Markdown parse mode.
func userMention(u *models.User) string {
	return fmt.Sprintf("[%s](tg://user?id=%d)", mdEscaper.Replace(userTitle(u)), u.ID)
}

// answer replies to a callback query; failures are only logged.
func answer(ctx context.Context, b *bot.Bot, queryID, text string) {
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: queryID,
		Text:            text,
	})
	if err != nil {
		slog.Warn("answer callback failed", "err", err)
	}
}

// deleteMessages removes messages, tolerating already-deleted ones.
func deleteMessages(ctx context.Context, b *bot.Bot, chatID int64, messageIDs ...int) {
	for _, id := range messageIDs {
		if id == 0 {
			continue
		}
		_, err := tgCall(ctx, "deleteMessage", func(ctx context.Context) (bool, error) {
			return b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: id})
		})
		if err != nil {
			slog.Debug("delete message failed", "chat_id", chatID, "message_id", id, "err", err)
		}
	}
}

// deleteSessionMessages cleans everything a session tracked.
func deleteSessionMessages(ctx context.Context, b *bot.Bot, s *storage.Session) {
	deleteMessages(ctx, b, s.ChatID, s.MessageIDs.Captcha)
}

// memberUser extracts the affected user from a chat member state.
func memberUser(m models.ChatMember) *models.User {
	switch m.Type {
	case models.ChatMemberTypeOwner:
		return m.Owner.User
	case models.ChatMemberTypeAdministrator:
		return &m.Administrator.User
	case models.ChatMemberTypeMember:
		return m.Member.User
	case models.ChatMemberTypeRestricted:
		return m.Restricted.User
	case models.ChatMemberTypeLeft:
		return m.Left.User
	case models.ChatMemberTypeBanned:
		return m.Banned.User
	}
	return nil
}

// isMember reports whether the state means "present in the chat".
func isMember(m models.ChatMember) bool {
	switch m.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return true
	case models.ChatMemberTypeRestricted:
		return m.Restricted.IsMember
	}
	return false
}
