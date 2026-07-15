package storage

// These tests need a real MongoDB. Point MONGO_TEST_URI at one, e.g.:
//
//	docker run --rm -p 27017:27017 mongo:7
//	MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/storage/
//
// Without MONGO_TEST_URI they are skipped.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("MONGO_TEST_URI not set")
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	dbName := fmt.Sprintf("protectron_test_%d", time.Now().UnixNano())
	db := client.Database(dbName)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	s := New(db)
	if err := s.EnsureIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func testSession(id string, chatID, userID int64) *Session {
	return &Session{
		ID:      id,
		ChatID:  chatID,
		UserID:  userID,
		Answer:  []string{"б", "в", "г", "б"},
		Input:   []string{},
		Buttons: map[string]string{"a1b2c3d4": "б", "e5f6a7b8": "в"},
		Attempt: 1,
		MessageIDs: MessageIDs{
			Captcha: 100,
		},
		ExpiresAt: time.Now().Add(5 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		DebugID:   "testchat-(@testuser)",
	}
}

func TestChatDefaultsAndEnsure(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	got, err := s.Chats.Get(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultChatSettings(42)
	if *got != *want {
		t.Errorf("defaults: got %+v want %+v", got, want)
	}

	ensured, err := s.Chats.Ensure(ctx, 42, "My Chat")
	if err != nil {
		t.Fatal(err)
	}
	if ensured.Title != "My Chat" || ensured.Lang != "ru" || ensured.CaptchaLength != 8 {
		t.Errorf("ensure: got %+v", ensured)
	}

	if err := s.Chats.SetField(ctx, 42, "lang", "en"); err != nil {
		t.Fatal(err)
	}
	got, err = s.Chats.Get(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lang != "en" || got.Title != "My Chat" || got.MaxAttempts != 2 {
		t.Errorf("after SetField: got %+v", got)
	}
}

func TestSetFieldUpsertsDefaults(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.Chats.SetField(ctx, 7, "captcha_length", 6); err != nil {
		t.Fatal(err)
	}
	got, err := s.Chats.Get(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.CaptchaLength != 6 || got.Lang != "ru" || got.CaptchaTimeoutSec != 300 {
		t.Errorf("got %+v", got)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := testSession("0011223344556677", 1, 2)
	if err := s.Sessions.Insert(ctx, sess); err != nil {
		t.Fatal(err)
	}

	got, err := s.Sessions.Get(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ChatID != 1 || got.UserID != 2 || got.Buttons["a1b2c3d4"] != "б" || len(got.Answer) != 4 {
		t.Errorf("got %+v", got)
	}

	if _, err := s.Sessions.Get(ctx, "ffffffffffffffff"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing id: got %v, want ErrNotFound", err)
	}

	byUser, err := s.Sessions.GetByChatUser(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if byUser.ID != sess.ID {
		t.Errorf("got %q", byUser.ID)
	}
}

func TestSessionUniquePerChatUser(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.Sessions.Insert(ctx, testSession("0000000000000001", 1, 2)); err != nil {
		t.Fatal(err)
	}
	err := s.Sessions.Insert(ctx, testSession("0000000000000002", 1, 2))
	if !IsDuplicate(err) {
		t.Errorf("second insert: got %v, want duplicate error", err)
	}
	// Different user in the same chat is fine.
	if err := s.Sessions.Insert(ctx, testSession("0000000000000003", 1, 3)); err != nil {
		t.Errorf("other user: %v", err)
	}
}

func TestInputPushPop(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := testSession("0000000000000010", 1, 2)
	if err := s.Sessions.Insert(ctx, sess); err != nil {
		t.Fatal(err)
	}

	got, err := s.Sessions.PushInput(ctx, sess.ID, "б")
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Sessions.PushInput(ctx, sess.ID, "в")
	if err != nil {
		t.Fatal(err)
	}
	if got.InputString() != "бв" {
		t.Errorf("input: %q", got.InputString())
	}
	if got.InputComplete() {
		t.Error("2 of 4 chars should not be complete")
	}

	got, err = s.Sessions.PopInput(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputString() != "б" {
		t.Errorf("after pop: %q", got.InputString())
	}

	// Pop to empty and once more: must stay empty, not error.
	if _, err = s.Sessions.PopInput(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.Sessions.PopInput(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Input) != 0 {
		t.Errorf("after popping empty: %v", got.Input)
	}

	if _, err := s.Sessions.PushInput(ctx, "ffffffffffffffff", "б"); !errors.Is(err, ErrNotFound) {
		t.Errorf("push to missing: got %v", err)
	}
}

func TestInputCorrect(t *testing.T) {
	sess := testSession("x", 1, 2)
	sess.Input = []string{"б", "в", "г", "б"}
	if !sess.InputCorrect() {
		t.Error("exact match should be correct")
	}
	sess.Input = []string{"б", "в", "г", "в"}
	if sess.InputCorrect() {
		t.Error("mismatch should be incorrect")
	}
}

func TestNewAttempt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := testSession("0000000000000020", 1, 2)
	if err := s.Sessions.Insert(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sessions.PushInput(ctx, sess.ID, "б"); err != nil {
		t.Fatal(err)
	}

	newExpiry := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond)
	got, err := s.Sessions.NewAttempt(ctx, sess.ID,
		[]string{"ж", "з"},
		map[string]string{"11112222": "ж", "33334444": "з"},
		200, newExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != 2 {
		t.Errorf("attempt: %d", got.Attempt)
	}
	if len(got.Input) != 0 {
		t.Errorf("input not reset: %v", got.Input)
	}
	if got.Answer[0] != "ж" || got.Buttons["11112222"] != "ж" {
		t.Errorf("answer/buttons not swapped: %+v", got)
	}
	if got.MessageIDs.Captcha != 200 || got.MessageIDs.Join != 0 {
		t.Errorf("message ids: %+v", got.MessageIDs)
	}
	if !got.ExpiresAt.Equal(newExpiry) {
		t.Errorf("expiry: %v want %v", got.ExpiresAt, newExpiry)
	}
}

func TestJoinMessageID(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := testSession("0000000000000030", 1, 2)
	if err := s.Sessions.Insert(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions.SetJoinMessageID(ctx, 1, 2, 99); err != nil {
		t.Fatal(err)
	}
	got, err := s.Sessions.Get(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MessageIDs.Join != 99 || got.MessageIDs.Captcha != 100 {
		t.Errorf("message ids: %+v", got.MessageIDs)
	}

	if err := s.Sessions.SetJoinMessageID(ctx, 1, 999, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("no session: got %v", err)
	}
}

func TestDeleteByChatUser(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := testSession("0000000000000040", 1, 2)
	if err := s.Sessions.Insert(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, err := s.Sessions.DeleteByChatUser(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != sess.ID {
		t.Errorf("got %q", got.ID)
	}
	if _, err := s.Sessions.Get(ctx, sess.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("should be gone: %v", err)
	}
	if _, err := s.Sessions.DeleteByChatUser(ctx, 1, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: got %v", err)
	}
}

func TestListExpired(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	expired := testSession("0000000000000050", 1, 2)
	expired.ExpiresAt = now.Add(-time.Minute)
	active := testSession("0000000000000051", 1, 3)
	active.ExpiresAt = now.Add(time.Minute)
	for _, sess := range []*Session{expired, active} {
		if err := s.Sessions.Insert(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Sessions.ListExpired(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != expired.ID {
		t.Errorf("got %d sessions", len(got))
	}
}

func TestStats(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.Stats.Inc(ctx, 1, StatJoins); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Stats.Inc(ctx, 1, StatPassed); err != nil {
		t.Fatal(err)
	}
	if err := s.Stats.Inc(ctx, 2, StatTimeouts); err != nil {
		t.Fatal(err)
	}

	totals, err := s.Stats.TotalsSince(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 {
		t.Fatalf("chats: %d", len(totals))
	}
	if totals[0].ChatID != 1 || totals[0].Joins != 3 || totals[0].Passed != 1 || totals[0].Failed != 0 {
		t.Errorf("chat 1: %+v", totals[0])
	}
	if totals[1].ChatID != 2 || totals[1].Timeouts != 1 {
		t.Errorf("chat 2: %+v", totals[1])
	}

	// A since-day in the future excludes today's buckets.
	future, err := s.Stats.TotalsSince(ctx, Day(time.Now().AddDate(0, 0, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(future) != 0 {
		t.Errorf("future window: %+v", future)
	}
	// A since-day in the past includes them.
	week, err := s.Stats.TotalsSince(ctx, Day(time.Now().AddDate(0, 0, -7)))
	if err != nil {
		t.Fatal(err)
	}
	if len(week) != 2 {
		t.Errorf("week window: %+v", week)
	}
}
