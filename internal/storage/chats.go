package storage

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ChatSettings struct {
	ID                int64     `bson:"_id"`
	Title             string    `bson:"title"`
	Lang              string    `bson:"lang"`
	CaptchaTimeoutSec int       `bson:"captcha_timeout_sec"`
	CaptchaLength     int       `bson:"captcha_length"`
	MaxAttempts       int       `bson:"max_attempts"`
	BanDurationSec    int       `bson:"ban_duration_sec"`
	UpdatedAt         time.Time `bson:"updated_at"`
}

func (c *ChatSettings) CaptchaTimeout() time.Duration {
	return time.Duration(c.CaptchaTimeoutSec) * time.Second
}

func (c *ChatSettings) BanDuration() time.Duration {
	return time.Duration(c.BanDurationSec) * time.Second
}

func DefaultChatSettings(chatID int64) *ChatSettings {
	return &ChatSettings{
		ID:                chatID,
		Lang:              "ru",
		CaptchaTimeoutSec: 300,
		CaptchaLength:     8,
		MaxAttempts:       2,
		BanDurationSec:    60,
	}
}

type ChatRepo struct {
	coll *mongo.Collection
}

// Get returns the chat settings, or the defaults when no document exists.
// It never writes: documents are created lazily by Ensure or SetField.
func (r *ChatRepo) Get(ctx context.Context, chatID int64) (*ChatSettings, error) {
	var s ChatSettings
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: chatID}}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return DefaultChatSettings(chatID), nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Ensure upserts the chat document (defaults on insert) and refreshes the
// denormalized title. Returns the resulting settings.
func (r *ChatRepo) Ensure(ctx context.Context, chatID int64, title string) (*ChatSettings, error) {
	d := DefaultChatSettings(chatID)
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "title", Value: title},
			{Key: "updated_at", Value: time.Now().UTC()},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "lang", Value: d.Lang},
			{Key: "captcha_timeout_sec", Value: d.CaptchaTimeoutSec},
			{Key: "captcha_length", Value: d.CaptchaLength},
			{Key: "max_attempts", Value: d.MaxAttempts},
			{Key: "ban_duration_sec", Value: d.BanDurationSec},
		}},
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var s ChatSettings
	if err := r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: chatID}}, update, opts).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SetField updates one settings field (upserting defaults for the rest).
// The field name must be validated by the caller (/set command allowlist).
func (r *ChatRepo) SetField(ctx context.Context, chatID int64, field string, value any) error {
	d := DefaultChatSettings(chatID)
	onInsert := bson.D{}
	for _, f := range []struct {
		name string
		val  any
	}{
		{"lang", d.Lang},
		{"captcha_timeout_sec", d.CaptchaTimeoutSec},
		{"captcha_length", d.CaptchaLength},
		{"max_attempts", d.MaxAttempts},
		{"ban_duration_sec", d.BanDurationSec},
	} {
		if f.name != field {
			onInsert = append(onInsert, bson.E{Key: f.name, Value: f.val})
		}
	}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: field, Value: value},
			{Key: "updated_at", Value: time.Now().UTC()},
		}},
		{Key: "$setOnInsert", Value: onInsert},
	}
	_, err := r.coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: chatID}}, update, options.UpdateOne().SetUpsert(true))
	return err
}
