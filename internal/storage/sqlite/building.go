package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"foxlogi/internal/building"
)

// AddBuilding inserts a new building and returns it with its assigned ID.
func (r *Repo) AddBuilding(ctx context.Context, b building.Building) (building.Building, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO buildings (guild_id, label, hexagon, town, type, role, password)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.GuildID, b.Label, b.Hexagon, b.Town, b.Type, b.Role, b.Password)
	if err != nil {
		return building.Building{}, fmt.Errorf("insert building: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return building.Building{}, fmt.Errorf("last insert id: %w", err)
	}
	b.ID = id
	return b, nil
}

// ListBuildingsByGuild returns the guild's buildings ordered by label.
func (r *Repo) ListBuildingsByGuild(ctx context.Context, guildID string) ([]building.Building, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, guild_id, label, hexagon, town, type, role, password
		 FROM buildings WHERE guild_id = ? ORDER BY label ASC`, guildID)
	if err != nil {
		return nil, fmt.Errorf("query buildings: %w", err)
	}
	defer rows.Close()

	var out []building.Building
	for rows.Next() {
		var b building.Building
		if err := rows.Scan(&b.ID, &b.GuildID, &b.Label, &b.Hexagon, &b.Town, &b.Type, &b.Role, &b.Password); err != nil {
			return nil, fmt.Errorf("scan building: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBuilding returns a building scoped to the guild; found is false if absent.
func (r *Repo) GetBuilding(ctx context.Context, guildID string, id int64) (building.Building, bool, error) {
	var b building.Building
	err := r.db.QueryRowContext(ctx,
		`SELECT id, guild_id, label, hexagon, town, type, role, password
		 FROM buildings WHERE id = ? AND guild_id = ?`, id, guildID).
		Scan(&b.ID, &b.GuildID, &b.Label, &b.Hexagon, &b.Town, &b.Type, &b.Role, &b.Password)
	if errors.Is(err, sql.ErrNoRows) {
		return building.Building{}, false, nil
	}
	if err != nil {
		return building.Building{}, false, fmt.Errorf("get building: %w", err)
	}
	return b, true, nil
}

// DeleteBuilding removes a building by ID within the guild. Removing a missing
// ID is not an error.
func (r *Repo) DeleteBuilding(ctx context.Context, guildID string, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM buildings WHERE id = ? AND guild_id = ?`, id, guildID); err != nil {
		return fmt.Errorf("delete building: %w", err)
	}
	return nil
}

// ListBuildingRoles returns the role ids allowed to view building codes.
func (r *Repo) ListBuildingRoles(ctx context.Context, guildID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT role_id FROM building_roles WHERE guild_id = ? ORDER BY role_id ASC`, guildID)
	if err != nil {
		return nil, fmt.Errorf("query building roles: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan building role: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddBuildingRole authorises a role to view building codes (idempotent).
func (r *Repo) AddBuildingRole(ctx context.Context, guildID, roleID string) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO building_roles (guild_id, role_id) VALUES (?, ?)
		 ON CONFLICT(guild_id, role_id) DO NOTHING`, guildID, roleID); err != nil {
		return fmt.Errorf("add building role: %w", err)
	}
	return nil
}

// RemoveBuildingRole revokes a role's access to building codes.
func (r *Repo) RemoveBuildingRole(ctx context.Context, guildID, roleID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM building_roles WHERE guild_id = ? AND role_id = ?`, guildID, roleID); err != nil {
		return fmt.Errorf("remove building role: %w", err)
	}
	return nil
}
