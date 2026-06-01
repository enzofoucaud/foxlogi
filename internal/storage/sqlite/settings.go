package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"foxlogi/internal/guildconfig"
)

// Get returns the guild's settings, or a zero-value Settings if none is stored.
func (r *Repo) Get(ctx context.Context, guildID string) (guildconfig.Settings, error) {
	s := guildconfig.Settings{GuildID: guildID}
	err := r.db.QueryRowContext(ctx,
		`SELECT craft_channel_id, request_channel_id FROM guild_settings WHERE guild_id = ?`,
		guildID).Scan(&s.CraftChannelID, &s.RequestChannelID)
	if errors.Is(err, sql.ErrNoRows) {
		return guildconfig.Settings{GuildID: guildID}, nil
	}
	if err != nil {
		return guildconfig.Settings{}, fmt.Errorf("get guild settings: %w", err)
	}
	return s, nil
}

// SetCraftChannel upserts the craft channel for the guild, leaving the request
// channel untouched.
func (r *Repo) SetCraftChannel(ctx context.Context, guildID, channelID string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, craft_channel_id) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET craft_channel_id = excluded.craft_channel_id`,
		guildID, channelID); err != nil {
		return fmt.Errorf("set craft channel: %w", err)
	}
	return nil
}

// SetRequestChannel upserts the request channel for the guild, leaving the craft
// channel untouched.
func (r *Repo) SetRequestChannel(ctx context.Context, guildID, channelID string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, request_channel_id) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET request_channel_id = excluded.request_channel_id`,
		guildID, channelID); err != nil {
		return fmt.Errorf("set request channel: %w", err)
	}
	return nil
}
