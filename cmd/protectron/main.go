package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/ice2heart/protectron/internal/config"
	"github.com/ice2heart/protectron/internal/handlers"
	"github.com/ice2heart/protectron/internal/i18n"
	"github.com/ice2heart/protectron/internal/storage"
	"github.com/ice2heart/protectron/internal/sweeper"
)

// Build metadata, set via -ldflags by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Load a .env file if present so the bot can run outside Docker.
	// A missing file is not an error (env vars may come from the environment).
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})))
	slog.Info("protectron", "version", version, "commit", commit, "date", date)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	msgs, err := i18n.Load("templates")
	if err != nil {
		return err
	}

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(shutdownCtx); err != nil {
			slog.Error("mongo disconnect failed", "err", err)
		}
	}()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return err
	}
	slog.Info("mongo connected", "db", cfg.MongoDB)

	store := storage.New(client.Database(cfg.MongoDB))
	if err := store.EnsureIndexes(ctx); err != nil {
		return err
	}

	h := handlers.New(store, msgs, cfg.AdminID)

	b, err := bot.New(cfg.APIToken,
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query", "chat_member"}),
		bot.WithDefaultHandler(func(context.Context, *bot.Bot, *models.Update) {}),
		bot.WithMessageTextHandler("/ping", bot.MatchTypePrefix, h.Ping),
		// Note: handler patterns must not overlap ("/set " vs "/settings"),
		// the framework matches them in random (map) order.
		bot.WithMessageTextHandler("/settings", bot.MatchTypePrefix, h.Settings),
		bot.WithMessageTextHandler("/stats", bot.MatchTypePrefix, h.Stats),
		bot.WithMessageTextHandler("/set ", bot.MatchTypePrefix, h.Set),
		bot.WithCallbackQueryDataHandler("c:", bot.MatchTypePrefix, h.Callback),
	)
	if err != nil {
		return err
	}
	b.RegisterHandlerMatchFunc(handlers.MatchChatMember, h.ChatMemberUpdate)
	b.RegisterHandlerMatchFunc(handlers.MatchNewChatMembers, h.NewChatMembers)
	b.RegisterHandlerMatchFunc(handlers.MatchLeftChatMember, h.LeftChatMember)

	me, err := b.GetMe(ctx)
	if err != nil {
		return err
	}
	slog.Info("bot started", "username", me.Username, "id", me.ID)

	go sweeper.Run(ctx, sweeper.DefaultInterval, store, b, h)

	b.Start(ctx)
	slog.Info("shutting down")
	return nil
}
