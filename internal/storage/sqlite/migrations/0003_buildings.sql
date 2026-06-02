-- +goose Up
CREATE TABLE buildings (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	guild_id  TEXT NOT NULL,
	label     TEXT NOT NULL,
	hexagon   TEXT NOT NULL,
	town      TEXT NOT NULL,
	type      TEXT NOT NULL,
	role      TEXT NOT NULL,
	password  TEXT NOT NULL
);
CREATE INDEX idx_buildings_guild ON buildings(guild_id);

CREATE TABLE building_roles (
	guild_id TEXT NOT NULL,
	role_id  TEXT NOT NULL,
	PRIMARY KEY (guild_id, role_id)
);

-- +goose Down
DROP TABLE building_roles;
DROP TABLE buildings;
