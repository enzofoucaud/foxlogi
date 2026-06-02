// Command foxlogi runs the Foxhole logistics Discord bot.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"foxlogi/internal/bot"
	"foxlogi/internal/config"
	"foxlogi/internal/storage/sqlite"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	level, known := levelFromString(cfg.LogLevel)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if !known {
		slog.Warn("unknown LOG_LEVEL, defaulting to info", "value", cfg.LogLevel)
	}
	slog.Info("starting foxlogi", "db_path", cfg.DBPath, "soon_threshold", cfg.SoonThreshold, "log_level", level)

	repo, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open storage", "err", err)
		os.Exit(1)
	}
	defer repo.Close()

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		slog.Error("create discord session", "err", err)
		os.Exit(1)
	}
	session.Identify.Intents = discordgo.IntentsGuilds

	// The registry is the single extension point: add a command by passing
	// another bot.Command here.
	registry := bot.NewRegistry(
		bot.NewCraftCommand(repo, repo, repo, cfg.SoonThreshold),
		bot.NewRequestCommand(repo, repo, repo),
		bot.NewConfigCommand(repo, repo),
		bot.NewBuildingCommand(repo, repo, repo),
	)
	// /help lists the registry's own commands, so it is added afterwards.
	registry.Add(bot.NewHelpCommand(registry))

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		registry.Dispatch(s, i)
	})
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		if s.State == nil || s.State.User == nil {
			slog.Warn("ready received but session user is not populated; skipping registration")
			return
		}
		slog.Info("connected", "user", r.User.Username, "guilds", len(r.Guilds))
		for _, g := range r.Guilds {
			if err := registry.Register(s, g.ID); err != nil {
				slog.Error("register commands", "guild", g.ID, "err", err)
			}
		}
	})

	if err := session.Open(); err != nil {
		slog.Error("open discord", "err", err)
		os.Exit(1)
	}
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := bot.NewScheduler(repo, repo, repo, session, time.Minute)
	go scheduler.Run(ctx)

	slog.Info("foxlogi is running; press Ctrl+C to stop")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
}

// levelFromString maps a LOG_LEVEL value to an slog.Level; ok is false for an
// unrecognised value (the caller defaults to info and warns).
func levelFromString(s string) (level slog.Level, ok bool) {
	switch s {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}
