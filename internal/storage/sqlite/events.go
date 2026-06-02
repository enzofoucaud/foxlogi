package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// RecordEvent appends one activity event. The payload is JSON-encoded; the
// timestamp is set here. It satisfies events.Recorder.
func (r *Repo) RecordEvent(ctx context.Context, guildID, userID, eventType string, payload map[string]string) error {
	encoded := ""
	if len(payload) > 0 {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal event payload: %w", err)
		}
		encoded = string(b)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO events (guild_id, user_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		guildID, userID, eventType, encoded, time.Now().Unix()); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}
