// Package guildconfig defines per-guild bot settings and the storage-agnostic
// Repository interface the bot depends on.
package guildconfig

import "context"

// Settings holds a guild's configurable options. An empty channel id means the
// feature falls back to the channel where the command was run.
type Settings struct {
	GuildID          string
	CraftChannelID   string
	RequestChannelID string
}

// Repository persists per-guild settings. Implementations must be safe for
// concurrent use.
type Repository interface {
	// Get returns the guild's settings. A guild with no row yields a zero-value
	// Settings (empty channel ids), not an error.
	Get(ctx context.Context, guildID string) (Settings, error)
	// SetCraftChannel sets the channel where craft messages are posted.
	SetCraftChannel(ctx context.Context, guildID, channelID string) error
	// SetRequestChannel sets the channel where request messages are posted.
	SetRequestChannel(ctx context.Context, guildID, channelID string) error

	// ListBuildingRoles returns the role ids allowed to view building codes.
	ListBuildingRoles(ctx context.Context, guildID string) ([]string, error)
	// AddBuildingRole authorises a role to view building codes (idempotent).
	AddBuildingRole(ctx context.Context, guildID, roleID string) error
	// RemoveBuildingRole revokes a role's access to building codes.
	RemoveBuildingRole(ctx context.Context, guildID, roleID string) error

	// Close releases underlying resources.
	Close() error
}
