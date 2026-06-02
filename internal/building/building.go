// Package building defines the domain model for stored building access codes
// and the storage-agnostic Repository interface the bot depends on.
package building

import "context"

// Building is a recorded base/depot with its access code. The Role field is
// descriptive (the building's function), unrelated to Discord roles.
type Building struct {
	ID       int64
	GuildID  string
	Label    string
	Hexagon  string
	Town     string
	Type     string
	Role     string
	Password string
}

// Repository persists building entries. Implementations must be safe for
// concurrent use. Method names are building-specific so a single store type can
// also satisfy the other domains' repositories.
type Repository interface {
	// AddBuilding stores a new building and returns it with its assigned ID.
	AddBuilding(ctx context.Context, b Building) (Building, error)
	// ListBuildingsByGuild returns the guild's buildings, ordered by label.
	ListBuildingsByGuild(ctx context.Context, guildID string) ([]Building, error)
	// GetBuilding returns a building scoped to the guild; found is false if absent.
	GetBuilding(ctx context.Context, guildID string, id int64) (b Building, found bool, err error)
	// DeleteBuilding removes a building by ID within the guild. Removing a
	// missing ID is not an error.
	DeleteBuilding(ctx context.Context, guildID string, id int64) error
	// Close releases underlying resources.
	Close() error
}
