package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"foxlogi/internal/craft"

	"github.com/bwmarrin/discordgo"
)

// Scheduler periodically notifies and removes crafts that have become ready.
type Scheduler struct {
	repo     craft.Repository
	session  *discordgo.Session
	interval time.Duration
}

// NewScheduler creates a Scheduler that checks for due crafts every interval.
func NewScheduler(repo craft.Repository, s *discordgo.Session, interval time.Duration) *Scheduler {
	return &Scheduler{repo: repo, session: s, interval: interval}
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
		if _, err := sc.session.ChannelMessageSend(c.ChannelID, msg); err != nil {
			log.Printf("scheduler: notify craft %d: %v", c.ID, err)
		}
		// Remove regardless of send outcome so a deleted channel or revoked
		// permission can't trap the craft in an infinite retry loop.
		if err := sc.repo.Delete(ctx, c.ID); err != nil {
			log.Printf("scheduler: delete craft %d: %v", c.ID, err)
		}
	}
}
