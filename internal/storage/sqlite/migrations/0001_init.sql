-- +goose Up
-- Baseline schema. IF NOT EXISTS keeps this migration safe on databases that
-- already hold these tables from the pre-migration era; later migrations use
-- strict DDL.
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

-- +goose Down
DROP TABLE IF EXISTS request_items;
DROP TABLE IF EXISTS requests;
DROP TABLE IF EXISTS crafts;
