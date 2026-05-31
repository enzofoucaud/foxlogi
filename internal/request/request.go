// Package request defines the domain model for the logistics request board and
// the storage-agnostic Repository interface the bot depends on.
package request

import (
	"context"
	"time"
)

// Priority levels for a request, ordered from most to least urgent.
const (
	PriorityHigh   = "high"
	PriorityMedium = "medium"
	PriorityLow    = "low"
)

// Request is a logistics request posted on a guild. It groups one or more
// item lines to be delivered to a single location.
type Request struct {
	ID        int64
	GuildID   string
	ChannelID string
	UserID    string // the requester (owner)
	Location  string
	Priority  string     // PriorityHigh | PriorityMedium | PriorityLow
	Deadline  *time.Time // optional
	Items     []RequestItem
}

// RequestItem is a single needed item line within a Request.
type RequestItem struct {
	ID        int64
	RequestID int64
	Item      string
	Quantity  int
	Delivered int // may exceed Quantity (surplus is accepted)
}

// Remaining returns how much of the line is still needed (never negative).
func (it RequestItem) Remaining() int {
	if it.Delivered >= it.Quantity {
		return 0
	}
	return it.Quantity - it.Delivered
}

// Repository persists requests and their item lines. Implementations must be
// safe for concurrent use.
type Repository interface {
	// CreateRequest stores a new (item-less) request and returns it with its ID.
	CreateRequest(ctx context.Context, r Request) (Request, error)
	// GetRequest returns a request (with its items) scoped to the guild.
	// found is false if no such request exists in that guild.
	GetRequest(ctx context.Context, guildID string, id int64) (req Request, found bool, err error)
	// AddItem appends an item line to a request and returns it with its ID.
	AddItem(ctx context.Context, requestID int64, item string, quantity int) (RequestItem, error)
	// ListOpenByGuild returns the guild's open requests (each with its items),
	// ordered by priority then id.
	ListOpenByGuild(ctx context.Context, guildID string) ([]Request, error)
	// Fill adds amount to the matching item line (case-insensitive) of the
	// request and returns the updated line. found is false if no such line.
	Fill(ctx context.Context, requestID int64, item string, amount int) (it RequestItem, found bool, err error)
	// IsFulfilled reports whether the request has at least one line and every
	// line has delivered >= quantity.
	IsFulfilled(ctx context.Context, requestID int64) (bool, error)
	// DeleteRequest removes a request and (via cascade) its item lines,
	// reporting whether a row was actually deleted.
	DeleteRequest(ctx context.Context, id int64) (bool, error)
	// Close releases underlying resources.
	Close() error
}
