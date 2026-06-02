// Command foxlogi runs the Foxhole logistics Discord bot.
package main

import (
	"context"
	"log"
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
		log.Fatalf("config: %v", err)
	}

	repo, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer repo.Close()

	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalf("discord session: %v", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds

	// The registry is the single extension point: add a command by passing
	// another bot.Command here.
	registry := bot.NewRegistry(
		bot.NewCraftCommand(repo, repo, cfg.SoonThreshold),
		bot.NewRequestCommand(repo, repo),
		bot.NewConfigCommand(repo),
		bot.NewBuildingCommand(repo, repo),
	)
	// /help lists the registry's own commands, so it is added afterwards.
	registry.Add(bot.NewHelpCommand(registry))

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		registry.Dispatch(s, i)
	})
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		if s.State == nil || s.State.User == nil {
			log.Println("ready received but session user is not populated; skipping registration")
			return
		}
		log.Printf("connected as %s", r.User.Username)
		for _, g := range r.Guilds {
			if err := registry.Register(s, g.ID); err != nil {
				log.Printf("register commands for guild %s: %v", g.ID, err)
			}
		}
	})

	if err := session.Open(); err != nil {
		log.Fatalf("open discord: %v", err)
	}
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := bot.NewScheduler(repo, repo, session, time.Minute)
	go scheduler.Run(ctx)

	log.Println("foxlogi is running. Press Ctrl+C to stop.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
}
