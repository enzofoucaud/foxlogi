// Package craft defines the core domain model for tracked Foxhole crafts and
// the storage-agnostic Repository interface the rest of the bot depends on.
package craft

import (
	"context"
	"time"
)

// Craft is a single manually-tracked production job on a Discord guild.
type Craft struct {
	ID         int64
	GuildID    string
	ChannelID  string
	UserID     string
	Item       string
	Quantity   int
	Completion time.Time
}

// Remaining returns the time left until the craft is ready, relative to now.
// It is negative once the craft is due.
func (c Craft) Remaining(now time.Time) time.Duration {
	return c.Completion.Sub(now)
}

// Repository persists crafts. Implementations must be safe for concurrent use.
type Repository interface {
	// Add stores a new craft and returns it with its assigned ID.
	Add(ctx context.Context, c Craft) (Craft, error)
	// ListByGuild returns the guild's active crafts, soonest completion first.
	ListByGuild(ctx context.Context, guildID string) ([]Craft, error)
	// Due returns every craft whose completion time is at or before now.
	Due(ctx context.Context, now time.Time) ([]Craft, error)
	// Delete removes a craft by ID. Removing a missing ID is not an error.
	Delete(ctx context.Context, id int64) error
	// Close releases underlying resources.
	Close() error
}
