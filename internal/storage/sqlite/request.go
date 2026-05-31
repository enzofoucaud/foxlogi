package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"foxlogi/internal/request"
)

const requestSchema = `
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

func init() { registerMigration(requestSchema) }

// CreateRequest inserts a new item-less request and returns it with its ID.
func (r *Repo) CreateRequest(ctx context.Context, req request.Request) (request.Request, error) {
	var deadline sql.NullInt64
	if req.Deadline != nil {
		deadline = sql.NullInt64{Int64: req.Deadline.Unix(), Valid: true}
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO requests (guild_id, channel_id, user_id, location, priority, deadline)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.GuildID, req.ChannelID, req.UserID, req.Location, req.Priority, deadline)
	if err != nil {
		return request.Request{}, fmt.Errorf("insert request: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return request.Request{}, fmt.Errorf("last insert id: %w", err)
	}
	req.ID = id
	return req, nil
}

// GetRequest returns a request and its items, scoped to the guild.
func (r *Repo) GetRequest(ctx context.Context, guildID string, id int64) (request.Request, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, guild_id, channel_id, user_id, location, priority, deadline
		 FROM requests WHERE id = ? AND guild_id = ?`, id, guildID)
	req, err := scanRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return request.Request{}, false, nil
	}
	if err != nil {
		return request.Request{}, false, err
	}
	items, err := r.loadItems(ctx, req.ID)
	if err != nil {
		return request.Request{}, false, err
	}
	req.Items = items
	return req, true, nil
}

// AddItem adds an item line to a request and returns it. If a line with the
// same item name (case-insensitive) already exists, its quantity is increased
// instead — a request never holds duplicate item rows, which would otherwise
// let Fill match multiple rows.
func (r *Repo) AddItem(ctx context.Context, requestID int64, item string, quantity int) (request.RequestItem, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE request_items SET quantity = quantity + ? WHERE request_id = ? AND lower(item) = lower(?)`,
		quantity, requestID, item)
	if err != nil {
		return request.RequestItem{}, fmt.Errorf("merge request item: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		var it request.RequestItem
		if err := r.db.QueryRowContext(ctx,
			`SELECT id, request_id, item, quantity, delivered
			 FROM request_items WHERE request_id = ? AND lower(item) = lower(?)`,
			requestID, item).Scan(&it.ID, &it.RequestID, &it.Item, &it.Quantity, &it.Delivered); err != nil {
			return request.RequestItem{}, fmt.Errorf("reload merged item: %w", err)
		}
		return it, nil
	}

	res, err = r.db.ExecContext(ctx,
		`INSERT INTO request_items (request_id, item, quantity, delivered) VALUES (?, ?, ?, 0)`,
		requestID, item, quantity)
	if err != nil {
		return request.RequestItem{}, fmt.Errorf("insert request item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return request.RequestItem{}, fmt.Errorf("last insert id: %w", err)
	}
	return request.RequestItem{ID: id, RequestID: requestID, Item: item, Quantity: quantity}, nil
}

// ListOpenByGuild returns the guild's requests (with items), ordered by
// priority then id.
func (r *Repo) ListOpenByGuild(ctx context.Context, guildID string) ([]request.Request, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, guild_id, channel_id, user_id, location, priority, deadline
		 FROM requests WHERE guild_id = ?
		 ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, id ASC`,
		guildID)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()

	var reqs []request.Request
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range reqs {
		items, err := r.loadItems(ctx, reqs[i].ID)
		if err != nil {
			return nil, err
		}
		reqs[i].Items = items
	}
	return reqs, nil
}

// Fill adds amount to the matching (case-insensitive) item line and returns the
// updated line. found is false if no such line exists in the request.
func (r *Repo) Fill(ctx context.Context, requestID int64, item string, amount int) (request.RequestItem, bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE request_items SET delivered = delivered + ?
		 WHERE request_id = ? AND lower(item) = lower(?)`,
		amount, requestID, item)
	if err != nil {
		return request.RequestItem{}, false, fmt.Errorf("update delivery: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return request.RequestItem{}, false, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return request.RequestItem{}, false, nil
	}

	var it request.RequestItem
	err = r.db.QueryRowContext(ctx,
		`SELECT id, request_id, item, quantity, delivered
		 FROM request_items WHERE request_id = ? AND lower(item) = lower(?)`,
		requestID, item).Scan(&it.ID, &it.RequestID, &it.Item, &it.Quantity, &it.Delivered)
	if err != nil {
		return request.RequestItem{}, false, fmt.Errorf("reload item: %w", err)
	}
	return it, true, nil
}

// IsFulfilled reports whether the request has at least one line and no line is
// still below its quantity.
func (r *Repo) IsFulfilled(ctx context.Context, requestID int64) (bool, error) {
	var total, unfinished int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN delivered < quantity THEN 1 ELSE 0 END), 0)
		 FROM request_items WHERE request_id = ?`, requestID).Scan(&total, &unfinished)
	if err != nil {
		return false, fmt.Errorf("count items: %w", err)
	}
	return total > 0 && unfinished == 0, nil
}

// DeleteRequest removes a request (its item lines go via ON DELETE CASCADE) and
// reports whether a row was actually deleted, so concurrent callers don't both
// act on the same completion.
func (r *Repo) DeleteRequest(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM requests WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete request: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRequest(s rowScanner) (request.Request, error) {
	var (
		req      request.Request
		deadline sql.NullInt64
	)
	if err := s.Scan(&req.ID, &req.GuildID, &req.ChannelID, &req.UserID, &req.Location, &req.Priority, &deadline); err != nil {
		return request.Request{}, err
	}
	if deadline.Valid {
		t := time.Unix(deadline.Int64, 0)
		req.Deadline = &t
	}
	return req, nil
}

func (r *Repo) loadItems(ctx context.Context, requestID int64) ([]request.RequestItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, request_id, item, quantity, delivered
		 FROM request_items WHERE request_id = ? ORDER BY id ASC`, requestID)
	if err != nil {
		return nil, fmt.Errorf("query request items: %w", err)
	}
	defer rows.Close()

	var items []request.RequestItem
	for rows.Next() {
		var it request.RequestItem
		if err := rows.Scan(&it.ID, &it.RequestID, &it.Item, &it.Quantity, &it.Delivered); err != nil {
			return nil, fmt.Errorf("scan request item: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
