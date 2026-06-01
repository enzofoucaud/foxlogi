package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"foxlogi/internal/craft"
	"foxlogi/internal/guildconfig"

	"github.com/bwmarrin/discordgo"
)

// Scheduler periodically notifies and removes crafts that have become ready.
type Scheduler struct {
	repo     craft.Repository
	settings guildconfig.Repository
	session  *discordgo.Session
	interval time.Duration
}

// NewScheduler creates a Scheduler that checks for due crafts every interval,
// routing ready notifications through the guild's configured craft channel.
func NewScheduler(repo craft.Repository, settings guildconfig.Repository, s *discordgo.Session, interval time.Duration) *Scheduler {
	return &Scheduler{repo: repo, settings: settings, session: s, interval: interval}
}

// Run blocks until ctx is cancelled, checking for due crafts on every tick.
func (sc *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sc.notifyDue(ctx)
		}
	}
}

func (sc *Scheduler) notifyDue(ctx context.Context) {
	due, err := sc.repo.Due(ctx, time.Now())
	if err != nil {
		log.Printf("scheduler: fetch due crafts: %v", err)
		return
	}
	for _, c := range due {
		msg := fmt.Sprintf("✅ <@%s> your craft **%d× %s** is ready!", c.UserID, c.Quantity, c.Item)
		target := resolveChannel(ctx, sc.settings, c.GuildID, c.ChannelID,
			func(set guildconfig.Settings) string { return set.CraftChannelID })
		sendWithFallback(sc.session, target, c.ChannelID, msg)
		// Remove regardless of send outcome so a deleted channel or revoked
		// permission can't trap the craft in an infinite retry loop.
		if err := sc.repo.Delete(ctx, c.ID); err != nil {
			log.Printf("scheduler: delete craft %d: %v", c.ID, err)
		}
	}
}
