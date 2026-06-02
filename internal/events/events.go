// Package events defines the storage-agnostic recorder for the append-only
// activity log. It is capture-only: events are written here and queried later
// for stats; nothing in the bot reads them back yet.
package events

import "context"

// Recorder appends one activity event. Implementations must be safe for
// concurrent use. Recording is best-effort at the call sites: a returned error
// is logged, never surfaced to the user.
type Recorder interface {
	// RecordEvent stores an event for the guild/user with a type and a small
	// payload. The name is distinct so one store type can also satisfy the other
	// domains' repositories.
	RecordEvent(ctx context.Context, guildID, userID, eventType string, payload map[string]string) error
}
