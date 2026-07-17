package storage

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MessageIDs struct {
	Captcha int `bson:"captcha"`
}

type Session struct {
	ID      string            `bson:"_id"`
	ChatID  int64             `bson:"chat_id"`
	UserID  int64             `bson:"user_id"`
	Answer  []string          `bson:"answer"`
	Input   []string          `bson:"input"`
	Buttons map[string]string `bson:"buttons"` // opaque token -> char
	Attempt int               `bson:"attempt"`
	// MessageIDs is everything to delete when the session completes.
	MessageIDs MessageIDs `bson:"message_ids"`
	ExpiresAt  time.Time  `bson:"expires_at"`
	CreatedAt  time.Time  `bson:"created_at"`
	DebugID    string     `bson:"debug_id"`
}

// InputString joins the presses so far, for the "still inputting" toast.
func (s *Session) InputString() string {
	out := ""
	for _, c := range s.Input {
		out += c
	}
	return out
}

// InputComplete reports whether the user entered as many chars as the answer.
func (s *Session) InputComplete() bool {
	return len(s.Input) >= len(s.Answer)
}

// InputCorrect reports whether input matches the answer exactly.
func (s *Session) InputCorrect() bool {
	if len(s.Input) != len(s.Answer) {
		return false
	}
	for i := range s.Answer {
		if s.Input[i] != s.Answer[i] {
			return false
		}
	}
	return true
}

type SessionRepo struct {
	coll *mongo.Collection
}

func (r *SessionRepo) ensureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "chat_id", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// Plain index, deliberately NOT a TTL index: expiry has side
			// effects (kick, ban, message cleanup) the sweeper must run
			// before the document may be deleted.
			Keys: bson.D{{Key: "expires_at", Value: 1}},
		},
	})
	return err
}

func (r *SessionRepo) Insert(ctx context.Context, s *Session) error {
	_, err := r.coll.InsertOne(ctx, s)
	return err
}

func (r *SessionRepo) Get(ctx context.Context, id string) (*Session, error) {
	var s Session
	err := r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) GetByChatUser(ctx context.Context, chatID, userID int64) (*Session, error) {
	var s Session
	err := r.coll.FindOne(ctx, bson.D{
		{Key: "chat_id", Value: chatID},
		{Key: "user_id", Value: userID},
	}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PushInput atomically appends one char and returns the updated session.
func (r *SessionRepo) PushInput(ctx context.Context, id, char string) (*Session, error) {
	return r.findOneAndUpdate(ctx, id, bson.D{
		{Key: "$push", Value: bson.D{{Key: "input", Value: char}}},
	})
}

// PopInput atomically removes the last input char (no-op when empty) and
// returns the updated session.
func (r *SessionRepo) PopInput(ctx context.Context, id string) (*Session, error) {
	return r.findOneAndUpdate(ctx, id, bson.D{
		{Key: "$pop", Value: bson.D{{Key: "input", Value: 1}}},
	})
}

// NewAttempt swaps in a fresh captcha: new answer, buttons and captcha
// message, empty input, bumped attempt counter, reset expiry.
func (r *SessionRepo) NewAttempt(ctx context.Context, id string, answer []string, buttons map[string]string, captchaMessageID int, expiresAt time.Time) (*Session, error) {
	return r.findOneAndUpdate(ctx, id, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "answer", Value: answer},
			{Key: "buttons", Value: buttons},
			{Key: "input", Value: []string{}},
			{Key: "message_ids.captcha", Value: captchaMessageID},
			{Key: "expires_at", Value: expiresAt},
		}},
		{Key: "$inc", Value: bson.D{{Key: "attempt", Value: 1}}},
	})
}

func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	return err
}

// DeleteByChatUser removes and returns the user's active session, if any.
func (r *SessionRepo) DeleteByChatUser(ctx context.Context, chatID, userID int64) (*Session, error) {
	var s Session
	err := r.coll.FindOneAndDelete(ctx, bson.D{
		{Key: "chat_id", Value: chatID},
		{Key: "user_id", Value: userID},
	}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListExpired returns sessions whose expires_at is before now.
func (r *SessionRepo) ListExpired(ctx context.Context, now time.Time) ([]*Session, error) {
	cur, err := r.coll.Find(ctx, bson.D{
		{Key: "expires_at", Value: bson.D{{Key: "$lt", Value: now}}},
	})
	if err != nil {
		return nil, err
	}
	var out []*Session
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SessionRepo) findOneAndUpdate(ctx context.Context, id string, update bson.D) (*Session, error) {
	var s Session
	err := r.coll.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: id}},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
