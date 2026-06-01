-- +goose Up
CREATE TABLE guild_settings (
	guild_id           TEXT PRIMARY KEY,
	craft_channel_id   TEXT NOT NULL DEFAULT '',
	request_channel_id TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE guild_settings;
