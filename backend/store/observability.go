package store

import (
	"context"
	"encoding/json"
	"time"
)

func (s *Store) AppendActivity(ctx context.Context, userID int64, eventType, entityType, entityID string, details map[string]any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO activity_events (user_id, event_type, entity_type, entity_id, details_json)
VALUES (?, ?, ?, ?, ?)
`, userID, eventType, entityType, entityID, string(data))
	return err
}

func (s *Store) ListActivity(ctx context.Context, limit int) ([]ActivityEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, event_type, entity_type, entity_id, details_json, created_at
FROM activity_events
ORDER BY id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []ActivityEvent
	for rows.Next() {
		event, err := scanActivityEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) RecordMetric(ctx context.Context, name string, value time.Duration, count int64, details map[string]any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO workflow_metrics (name, value_ms, count, details_json)
VALUES (?, ?, ?, ?)
`, name, value.Milliseconds(), count, string(data))
	return err
}

func (s *Store) ListMetrics(ctx context.Context, limit int) ([]WorkflowMetric, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, value_ms, count, details_json, created_at
FROM workflow_metrics
ORDER BY id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []WorkflowMetric
	for rows.Next() {
		metric, err := scanWorkflowMetric(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, rows.Err()
}

func (s *Store) DebugSetting(ctx context.Context, envEnabled bool) (DebugSetting, error) {
	if envEnabled {
		return DebugSetting{Enabled: true, Source: "environment", UpdatedAt: time.Now().UTC()}, nil
	}
	var value string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT value, updated_at
FROM app_settings
WHERE key = 'debug_mode'
`).Scan(&value, &updatedAt)
	if err != nil {
		if notFound(err) {
			return DebugSetting{Enabled: false, Source: "default", UpdatedAt: time.Time{}}, nil
		}
		return DebugSetting{}, err
	}
	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return DebugSetting{}, err
	}
	return DebugSetting{Enabled: value == "true", Source: "admin", UpdatedAt: updated}, nil
}

func (s *Store) SetDebugMode(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('debug_mode', ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
`, value)
	return err
}

func (s *Store) AppendDebugTrace(ctx context.Context, correlationID, traceType string, payload map[string]any, retention time.Duration) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(retention).Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO debug_traces (correlation_id, trace_type, payload_json, expires_at)
VALUES (?, ?, ?, ?)
`, correlationID, traceType, string(data), expiresAt)
	return err
}

func (s *Store) PruneDebugTraces(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM debug_traces WHERE expires_at <= ?`, now.UTC().Format(time.RFC3339))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}
