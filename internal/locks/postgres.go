package locks

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresManager struct {
	db *sql.DB
}

func NewPostgresManager(ctx context.Context, dsn string) (*PostgresManager, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres locks db: %w", err)
	}
	manager := &PostgresManager{db: db}
	if err := manager.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return manager, nil
}

func (m *PostgresManager) Close() error {
	return m.db.Close()
}

func (m *PostgresManager) TryAcquire(ctx context.Context, key, owner string, lease time.Duration, metadata Metadata) (bool, error) {
	expiresAt := time.Now().UTC().Add(lease)
	result, err := m.db.ExecContext(ctx, `
		INSERT INTO stage_resource_locks (
			lock_key, owner_id, run_id, attempt_id, issue_id, stage, worker_id, acquired_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
		ON CONFLICT (lock_key) DO UPDATE SET
			owner_id = EXCLUDED.owner_id,
			run_id = EXCLUDED.run_id,
			attempt_id = EXCLUDED.attempt_id,
			issue_id = EXCLUDED.issue_id,
			stage = EXCLUDED.stage,
			worker_id = EXCLUDED.worker_id,
			acquired_at = NOW(),
			expires_at = EXCLUDED.expires_at
		WHERE stage_resource_locks.expires_at < NOW() OR stage_resource_locks.owner_id = EXCLUDED.owner_id
	`, key, owner, metadata.RunID, metadata.AttemptID, metadata.IssueID, metadata.Stage, metadata.WorkerID, expiresAt)
	if err != nil {
		return false, fmt.Errorf("acquire lock %s: %w", key, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("acquire lock rows affected %s: %w", key, err)
	}
	return rows > 0, nil
}

func (m *PostgresManager) Refresh(ctx context.Context, key, owner string, lease time.Duration) error {
	expiresAt := time.Now().UTC().Add(lease)
	_, err := m.db.ExecContext(ctx, `
		UPDATE stage_resource_locks
		SET expires_at = $3
		WHERE lock_key = $1 AND owner_id = $2
	`, key, owner, expiresAt)
	if err != nil {
		return fmt.Errorf("refresh lock %s: %w", key, err)
	}
	return nil
}

func (m *PostgresManager) Release(ctx context.Context, key, owner string) error {
	_, err := m.db.ExecContext(ctx, `
		DELETE FROM stage_resource_locks
		WHERE lock_key = $1 AND owner_id = $2
	`, key, owner)
	if err != nil {
		return fmt.Errorf("release lock %s: %w", key, err)
	}
	return nil
}

func (m *PostgresManager) init(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS stage_resource_locks (
			lock_key TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL,
			issue_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL
		);
		CREATE INDEX IF NOT EXISTS stage_resource_locks_expires_at_idx
			ON stage_resource_locks (expires_at);
	`)
	if err != nil {
		return fmt.Errorf("init postgres locks db: %w", err)
	}
	return nil
}
