package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"foxlogi/internal/craft"
)

const craftSchema = `
CREATE TABLE IF NOT EXISTS crafts (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	guild_id    TEXT    NOT NULL,
	channel_id  TEXT    NOT NULL,
	user_id     TEXT    NOT NULL,
	item        TEXT    NOT NULL,
	quantity    INTEGER NOT NULL,
	completion  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_crafts_guild ON crafts(guild_id);
CREATE INDEX IF NOT EXISTS idx_crafts_completion ON crafts(completion);
`

func init() { registerMigration(craftSchema) }

// Add inserts a new craft and returns it with its assigned ID.
func (r *Repo) Add(ctx context.Context, c craft.Craft) (craft.Craft, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO crafts (guild_id, channel_id, user_id, item, quantity, completion)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.GuildID, c.ChannelID, c.UserID, c.Item, c.Quantity, c.Completion.Unix())
	if err != nil {
		return craft.Craft{}, fmt.Errorf("insert craft: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return craft.Craft{}, fmt.Errorf("last insert id: %w", err)
	}
	c.ID = id
	return c, nil
}

// ListByGuild returns the guild's crafts sorted by soonest completion first.
func (r *Repo) ListByGuild(ctx context.Context, guildID string) ([]craft.Craft, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, guild_id, channel_id, user_id, item, quantity, completion
		 FROM crafts WHERE guild_id = ? ORDER BY completion ASC`, guildID)
	if err != nil {
		return nil, fmt.Errorf("query crafts: %w", err)
	}
	defer rows.Close()
	return scanCrafts(rows)
}

// Due returns every craft whose completion time is at or before now.
func (r *Repo) Due(ctx context.Context, now time.Time) ([]craft.Craft, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, guild_id, channel_id, user_id, item, quantity, completion
		 FROM crafts WHERE completion <= ? ORDER BY completion ASC`, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("query due crafts: %w", err)
	}
	defer rows.Close()
	return scanCrafts(rows)
}

// Delete removes a craft by ID. Removing a missing ID is not an error.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM crafts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete craft: %w", err)
	}
	return nil
}

func scanCrafts(rows *sql.Rows) ([]craft.Craft, error) {
	var out []craft.Craft
	for rows.Next() {
		var (
			c    craft.Craft
			unix int64
		)
		if err := rows.Scan(&c.ID, &c.GuildID, &c.ChannelID, &c.UserID, &c.Item, &c.Quantity, &unix); err != nil {
			return nil, fmt.Errorf("scan craft: %w", err)
		}
		c.Completion = time.Unix(unix, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}
