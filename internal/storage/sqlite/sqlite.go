// Package sqlite provides craft.Repository and request.Repository backed by
// SQLite via the pure-Go modernc.org/sqlite driver (no CGO required). A single
// *sql.DB is shared by both so the connection cap and pragmas hold across all
// features.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"foxlogi/internal/craft"

	_ "modernc.org/sqlite"
)

const schema = `
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

CREATE TABLE IF NOT EXISTS requests (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	guild_id    TEXT    NOT NULL,
	channel_id  TEXT    NOT NULL,
	user_id     TEXT    NOT NULL,
	location    TEXT    NOT NULL,
	priority    TEXT    NOT NULL,
	deadline    INTEGER
);
CREATE INDEX IF NOT EXISTS idx_requests_guild ON requests(guild_id);

CREATE TABLE IF NOT EXISTS request_items (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	request_id  INTEGER NOT NULL,
	item        TEXT    NOT NULL,
	quantity    INTEGER NOT NULL,
	delivered   INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_request_items_request ON request_items(request_id);
`

// Repo is a SQLite-backed craft.Repository.
type Repo struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and migrates the
// schema. It satisfies craft.Repository.
func Open(path string) (*Repo, error) {
	// busy_timeout lets writes wait instead of failing with SQLITE_BUSY; capping
	// the pool at a single connection serialises the scheduler goroutine and the
	// command handlers, eliminating "database is locked" under concurrent access.
	// foreign_keys enables ON DELETE CASCADE for request_items.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &Repo{db: db}, nil
}

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

// Close releases the database handle.
func (r *Repo) Close() error {
	return r.db.Close()
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
