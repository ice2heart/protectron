package storage

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Stat counter names; also the Mongo field names in the stats collection.
const (
	StatJoins    = "joins"    // captcha sessions started
	StatPassed   = "passed"   // captcha solved
	StatFailed   = "failed"   // kicked after the last wrong attempt
	StatTimeouts = "timeouts" // kicked by the sweeper
	StatLeaves   = "leaves"   // left or was removed mid-captcha
)

var statCounters = []string{StatJoins, StatPassed, StatFailed, StatTimeouts, StatLeaves}

// ChatTotals is the aggregated view for one chat.
type ChatTotals struct {
	ChatID   int64 `bson:"_id"`
	Joins    int64 `bson:"joins"`
	Passed   int64 `bson:"passed"`
	Failed   int64 `bson:"failed"`
	Timeouts int64 `bson:"timeouts"`
	Leaves   int64 `bson:"leaves"`
}

// StatsRepo stores per-chat, per-UTC-day counters:
// {_id: "<chat_id>:<YYYY-MM-DD>", chat_id, date, joins, passed, …}
type StatsRepo struct {
	coll *mongo.Collection
}

// Day formats t as the stats bucket key day.
func Day(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// Inc bumps one counter for today's bucket.
func (r *StatsRepo) Inc(ctx context.Context, chatID int64, counter string) error {
	day := Day(time.Now())
	_, err := r.coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: fmt.Sprintf("%d:%s", chatID, day)}},
		bson.D{
			{Key: "$inc", Value: bson.D{{Key: counter, Value: 1}}},
			{Key: "$setOnInsert", Value: bson.D{
				{Key: "chat_id", Value: chatID},
				{Key: "date", Value: day},
			}},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// TotalsSince sums counters per chat for buckets with date >= sinceDay.
// An empty sinceDay means all time. Sorted by chat id for stable output.
func (r *StatsRepo) TotalsSince(ctx context.Context, sinceDay string) ([]ChatTotals, error) {
	sums := bson.D{}
	for _, c := range statCounters {
		sums = append(sums, bson.E{Key: c, Value: bson.D{{Key: "$sum", Value: "$" + c}}})
	}
	pipeline := mongo.Pipeline{}
	if sinceDay != "" {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "date", Value: bson.D{{Key: "$gte", Value: sinceDay}}},
		}}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$group", Value: append(bson.D{{Key: "_id", Value: "$chat_id"}}, sums...)}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	)

	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var out []ChatTotals
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
