package signals

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open signals postgres db: %w", err)
	}
	store := &PostgresStore{db: db}
	if err := store.Init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS pipeline_events (
			id BIGSERIAL PRIMARY KEY,
			kind TEXT NOT NULL,
			run_id TEXT NOT NULL,
			issue_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			repo_scope TEXT NOT NULL,
			environment TEXT NOT NULL,
			service_scope TEXT NOT NULL,
			status TEXT NOT NULL,
			severity TEXT NOT NULL,
			summary TEXT NOT NULL,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS pipeline_events_stage_created_idx
			ON pipeline_events (stage, repo_scope, created_at DESC);
		CREATE TABLE IF NOT EXISTS operational_signals (
			id BIGSERIAL PRIMARY KEY,
			category TEXT NOT NULL,
			fingerprint TEXT NOT NULL UNIQUE,
			run_id TEXT NOT NULL,
			issue_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			repo_scope TEXT NOT NULL,
			environment TEXT NOT NULL,
			service_scope TEXT NOT NULL,
			severity TEXT NOT NULL,
			status TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			event_count INTEGER NOT NULL DEFAULT 1,
			last_event_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("init signals schema: %w", err)
	}
	return nil
}

func (s *PostgresStore) InsertEvent(ctx context.Context, event PipelineEvent) (PipelineEvent, error) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	metadata := map[string]any{}
	if event.Metadata != nil {
		metadata = event.Metadata
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return PipelineEvent{}, err
	}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO pipeline_events (
			kind, run_id, issue_id, attempt_id, stage, worker_id, repo_scope, environment,
			service_scope, status, severity, summary, duration_ms, cost_usd, metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id, created_at
	`, event.Kind, event.RunID, event.IssueID, event.AttemptID, event.Stage, event.WorkerID, event.RepoScope,
		event.Environment, event.ServiceScope, event.Status, event.Severity, event.Summary, event.DurationMS, event.CostUSD, payload, event.CreatedAt).
		Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return PipelineEvent{}, fmt.Errorf("insert pipeline event: %w", err)
	}
	return event, nil
}

func (s *PostgresStore) RecentEvents(ctx context.Context, kind, stage, repoScope, status string, since time.Time, limit int, excludeAttemptID string) ([]PipelineEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, run_id, issue_id, attempt_id, stage, worker_id, repo_scope, environment, service_scope,
		       status, severity, summary, duration_ms, cost_usd, metadata, created_at
		FROM pipeline_events
		WHERE kind = $1 AND stage = $2 AND repo_scope = $3 AND status = $4 AND created_at >= $5 AND attempt_id <> $6
		ORDER BY created_at DESC
		LIMIT $7
	`, kind, stage, repoScope, status, since, excludeAttemptID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}
	defer rows.Close()

	var out []PipelineEvent
	for rows.Next() {
		event, err := scanPipelineEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *PostgresStore) CountEvents(ctx context.Context, kind, stage, repoScope string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pipeline_events
		WHERE kind = $1 AND stage = $2 AND repo_scope = $3 AND created_at >= $4
	`, kind, stage, repoScope, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) UpsertSignal(ctx context.Context, signal OperationalSignal) (OperationalSignal, error) {
	if signal.LastEventAt.IsZero() {
		signal.LastEventAt = time.Now().UTC()
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO operational_signals (
			category, fingerprint, run_id, issue_id, stage, repo_scope, environment, service_scope,
			severity, status, title, summary, event_count, last_event_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,$13)
		ON CONFLICT (fingerprint) DO UPDATE SET
			run_id = EXCLUDED.run_id,
			issue_id = EXCLUDED.issue_id,
			stage = EXCLUDED.stage,
			repo_scope = EXCLUDED.repo_scope,
			environment = EXCLUDED.environment,
			service_scope = EXCLUDED.service_scope,
			severity = EXCLUDED.severity,
			status = EXCLUDED.status,
			title = EXCLUDED.title,
			summary = EXCLUDED.summary,
			event_count = operational_signals.event_count + 1,
			last_event_at = EXCLUDED.last_event_at,
			updated_at = NOW()
		RETURNING id, category, fingerprint, run_id, issue_id, stage, repo_scope, environment, service_scope,
		          severity, status, title, summary, event_count, last_event_at, created_at, updated_at
	`, signal.Category, signal.Fingerprint, signal.RunID, signal.IssueID, signal.Stage, signal.RepoScope,
		signal.Environment, signal.ServiceScope, signal.Severity, signal.Status, signal.Title, signal.Summary, signal.LastEventAt).
		Scan(&signal.ID, &signal.Category, &signal.Fingerprint, &signal.RunID, &signal.IssueID, &signal.Stage,
			&signal.RepoScope, &signal.Environment, &signal.ServiceScope, &signal.Severity, &signal.Status,
			&signal.Title, &signal.Summary, &signal.EventCount, &signal.LastEventAt, &signal.CreatedAt, &signal.UpdatedAt)
	if err != nil {
		return OperationalSignal{}, fmt.Errorf("upsert signal: %w", err)
	}
	return signal, nil
}

func (s *PostgresStore) ListSignals(ctx context.Context, req ListRequest) ([]OperationalSignal, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, category, fingerprint, run_id, issue_id, stage, repo_scope, environment, service_scope,
		       severity, status, title, summary, event_count, last_event_at, created_at, updated_at
		FROM operational_signals
		WHERE ($1 = '' OR repo_scope = $1)
		  AND ($2 = '' OR stage = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY updated_at DESC
		LIMIT $4
	`, req.RepoScope, req.Stage, req.Status, limit)
	if err != nil {
		return nil, fmt.Errorf("list signals: %w", err)
	}
	defer rows.Close()

	out := make([]OperationalSignal, 0, limit)
	for rows.Next() {
		var signal OperationalSignal
		if err := rows.Scan(&signal.ID, &signal.Category, &signal.Fingerprint, &signal.RunID, &signal.IssueID, &signal.Stage,
			&signal.RepoScope, &signal.Environment, &signal.ServiceScope, &signal.Severity, &signal.Status,
			&signal.Title, &signal.Summary, &signal.EventCount, &signal.LastEventAt, &signal.CreatedAt, &signal.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, signal)
	}
	return out, rows.Err()
}

func scanPipelineEvent(scanner interface {
	Scan(dest ...any) error
}) (PipelineEvent, error) {
	var event PipelineEvent
	var payload []byte
	if err := scanner.Scan(&event.ID, &event.Kind, &event.RunID, &event.IssueID, &event.AttemptID, &event.Stage,
		&event.WorkerID, &event.RepoScope, &event.Environment, &event.ServiceScope, &event.Status,
		&event.Severity, &event.Summary, &event.DurationMS, &event.CostUSD, &payload, &event.CreatedAt); err != nil {
		return PipelineEvent{}, err
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &event.Metadata)
	}
	return event, nil
}
