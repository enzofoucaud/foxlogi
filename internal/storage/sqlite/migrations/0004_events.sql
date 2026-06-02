-- +goose Up
CREATE TABLE events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	guild_id   TEXT    NOT NULL,
	user_id    TEXT    NOT NULL,
	type       TEXT    NOT NULL,
	payload    TEXT    NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE INDEX idx_events_guild_created ON events(guild_id, created_at);

-- +goose Down
DROP TABLE events;
