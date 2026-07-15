// Package storage holds the MongoDB repositories.
//
// Collections:
//   - chats:    per-chat settings, _id = telegram chat id
//   - sessions: one active captcha per (chat, user), _id = 16-hex session id
package storage

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = errors.New("storage: not found")

// IsDuplicate reports whether err is a unique-index violation.
func IsDuplicate(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}

type Store struct {
	Chats    *ChatRepo
	Sessions *SessionRepo
	Stats    *StatsRepo
}

func New(db *mongo.Database) *Store {
	return &Store{
		Chats:    &ChatRepo{coll: db.Collection("chats")},
		Sessions: &SessionRepo{coll: db.Collection("sessions")},
		Stats:    &StatsRepo{coll: db.Collection("stats")},
	}
}

// EnsureIndexes creates all indexes; call once on startup.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	return s.Sessions.ensureIndexes(ctx)
}
