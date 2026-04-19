package ratchet

import (
	"context"
	"database/sql"
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
		return nil, fmt.Errorf("open ratchet postgres db: %w", err)
	}
	store := &PostgresStore{db: db}
	if err := store.Init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS finding_events (
			id BIGSERIAL PRIMARY KEY,
			run_id TEXT NOT NULL,
			issue_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			repo_scope TEXT NOT NULL,
			environment TEXT NOT NULL,
			service_scope TEXT NOT NULL,
			scope_type TEXT NOT NULL,
			scope_key TEXT NOT NULL,
			category TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			severity TEXT NOT NULL,
			outcome_impact TEXT NOT NULL,
			evidence_uri TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS finding_clusters (
			id BIGSERIAL PRIMARY KEY,
			scope_type TEXT NOT NULL,
			scope_key TEXT NOT NULL,
			category TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			title TEXT NOT NULL,
			canonical_summary TEXT NOT NULL,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			occurrence_count INTEGER NOT NULL DEFAULT 0,
			distinct_run_count INTEGER NOT NULL DEFAULT 0,
			distinct_issue_count INTEGER NOT NULL DEFAULT 0,
			highest_severity TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			UNIQUE(scope_type, scope_key, category, fingerprint)
		);
		CREATE TABLE IF NOT EXISTS cluster_stage_stats (
			cluster_id BIGINT NOT NULL REFERENCES finding_clusters(id) ON DELETE CASCADE,
			stage TEXT NOT NULL,
			occurrence_count INTEGER NOT NULL DEFAULT 0,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			rollback_count INTEGER NOT NULL DEFAULT 0,
			block_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(cluster_id, stage)
		);
		CREATE TABLE IF NOT EXISTS invariant_proposals (
			id BIGSERIAL PRIMARY KEY,
			cluster_id BIGINT NOT NULL REFERENCES finding_clusters(id) ON DELETE CASCADE,
			scope_type TEXT NOT NULL,
			scope_key TEXT NOT NULL,
			category TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			proposed_text TEXT NOT NULL,
			rationale TEXT NOT NULL,
			evidence_summary TEXT NOT NULL,
			proposal_reason TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'proposed',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE IF NOT EXISTS active_invariants (
			id BIGSERIAL PRIMARY KEY,
			scope_type TEXT NOT NULL,
			scope_key TEXT NOT NULL,
			category TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			statement TEXT NOT NULL,
			severity TEXT NOT NULL,
			enforcement_mode TEXT NOT NULL,
			source_proposal_id BIGINT REFERENCES invariant_proposals(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(scope_type, scope_key, category, fingerprint)
		);
		CREATE TABLE IF NOT EXISTS invariant_stage_stats (
			invariant_id BIGINT NOT NULL REFERENCES active_invariants(id) ON DELETE CASCADE,
			stage TEXT NOT NULL,
			occurrence_count INTEGER NOT NULL DEFAULT 0,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			rollback_count INTEGER NOT NULL DEFAULT 0,
			block_count INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(invariant_id, stage)
		);
	`)
	if err != nil {
		return fmt.Errorf("init ratchet schema: %w", err)
	}
	return nil
}

func (s *PostgresStore) RecordFinding(ctx context.Context, event FindingEvent) (FindingCluster, *ActiveInvariant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FindingCluster{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO finding_events (
			run_id, issue_id, stage, repo_scope, environment, service_scope,
			scope_type, scope_key, category, fingerprint, title, summary,
			severity, outcome_impact, evidence_uri
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, event.RunID, event.IssueID, event.Stage, event.RepoScope, event.Environment, event.ServiceScope,
		event.ScopeType, event.ScopeKey, event.Category, event.Fingerprint, event.Title, event.Summary,
		event.Severity, event.OutcomeImpact, event.EvidenceURI); err != nil {
		return FindingCluster{}, nil, fmt.Errorf("insert finding event: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO finding_clusters (
			scope_type, scope_key, category, fingerprint, title, canonical_summary, highest_severity, occurrence_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,1)
		ON CONFLICT (scope_type, scope_key, category, fingerprint) DO UPDATE SET
			title = EXCLUDED.title,
			canonical_summary = EXCLUDED.canonical_summary,
			last_seen_at = NOW(),
			occurrence_count = finding_clusters.occurrence_count + 1,
			highest_severity = `+highestSeveritySQL("EXCLUDED.highest_severity", "finding_clusters.highest_severity")+`
	`, event.ScopeType, event.ScopeKey, event.Category, event.Fingerprint, event.Title, event.Summary, event.Severity); err != nil {
		return FindingCluster{}, nil, fmt.Errorf("upsert finding cluster: %w", err)
	}

	cluster, err := s.readClusterTx(ctx, tx, event)
	if err != nil {
		return FindingCluster{}, nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE finding_clusters
		SET distinct_run_count = (
				SELECT COUNT(DISTINCT run_id) FROM finding_events
				WHERE scope_type = $1 AND scope_key = $2 AND category = $3 AND fingerprint = $4
			),
			distinct_issue_count = (
				SELECT COUNT(DISTINCT issue_id) FROM finding_events
				WHERE scope_type = $1 AND scope_key = $2 AND category = $3 AND fingerprint = $4
			)
		WHERE id = $5
	`, event.ScopeType, event.ScopeKey, event.Category, event.Fingerprint, cluster.ID); err != nil {
		return FindingCluster{}, nil, fmt.Errorf("refresh cluster counts: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_stage_stats (cluster_id, stage, occurrence_count, rollback_count, block_count)
		VALUES ($1, $2, 1, $3, $4)
		ON CONFLICT (cluster_id, stage) DO UPDATE SET
			occurrence_count = cluster_stage_stats.occurrence_count + 1,
			last_seen_at = NOW(),
			rollback_count = cluster_stage_stats.rollback_count + EXCLUDED.rollback_count,
			block_count = cluster_stage_stats.block_count + EXCLUDED.block_count
	`, cluster.ID, event.Stage, boolInt(event.OutcomeImpact == "rollback"), boolInt(event.OutcomeImpact == "block")); err != nil {
		return FindingCluster{}, nil, fmt.Errorf("upsert cluster stage stat: %w", err)
	}

	activeInvariant, err := s.readActiveInvariantTx(ctx, tx, event.ScopeType, event.ScopeKey, event.Category, event.Fingerprint)
	if err != nil {
		return FindingCluster{}, nil, err
	}
	if activeInvariant != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invariant_stage_stats (invariant_id, stage, occurrence_count, rollback_count, block_count)
			VALUES ($1, $2, 1, $3, $4)
			ON CONFLICT (invariant_id, stage) DO UPDATE SET
				occurrence_count = invariant_stage_stats.occurrence_count + 1,
				last_seen_at = NOW(),
				rollback_count = invariant_stage_stats.rollback_count + EXCLUDED.rollback_count,
				block_count = invariant_stage_stats.block_count + EXCLUDED.block_count
		`, activeInvariant.ID, event.Stage, boolInt(event.OutcomeImpact == "rollback"), boolInt(event.OutcomeImpact == "block")); err != nil {
			return FindingCluster{}, nil, fmt.Errorf("upsert invariant stage stat: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return FindingCluster{}, nil, err
	}

	cluster, err = s.readCluster(ctx, cluster.ID)
	if err != nil {
		return FindingCluster{}, nil, err
	}
	return cluster, activeInvariant, nil
}

func (s *PostgresStore) HasOpenProposal(ctx context.Context, clusterID int64) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM invariant_proposals WHERE cluster_id = $1 AND status = 'proposed'
	`, clusterID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *PostgresStore) CreateProposal(ctx context.Context, cluster FindingCluster, reason string) (*InvariantProposal, error) {
	var proposal InvariantProposal
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO invariant_proposals (
			cluster_id, scope_type, scope_key, category, fingerprint, proposed_text, rationale, evidence_summary, proposal_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, cluster_id, scope_type, scope_key, category, fingerprint, proposed_text, rationale, evidence_summary, proposal_reason, status, created_at, updated_at
	`, cluster.ID, cluster.ScopeType, cluster.ScopeKey, cluster.Category, cluster.Fingerprint,
		statementForCluster(cluster), rationaleForCluster(cluster, reason), cluster.CanonicalSummary, reason).
		Scan(&proposal.ID, &proposal.ClusterID, &proposal.ScopeType, &proposal.ScopeKey, &proposal.Category, &proposal.Fingerprint,
			&proposal.ProposedText, &proposal.Rationale, &proposal.EvidenceSummary, &proposal.ProposalReason, &proposal.Status, &proposal.CreatedAt, &proposal.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}
	return &proposal, nil
}

func (s *PostgresStore) ActivateProposal(ctx context.Context, proposalID int64, enforcementMode string) (*ActiveInvariant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var proposal InvariantProposal
	var cluster FindingCluster
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, p.cluster_id, p.scope_type, p.scope_key, p.category, p.fingerprint, p.proposed_text, p.rationale, p.evidence_summary, p.proposal_reason, p.status, p.created_at, p.updated_at,
		       c.id, c.scope_type, c.scope_key, c.category, c.fingerprint, c.title, c.canonical_summary, c.first_seen_at, c.last_seen_at, c.occurrence_count, c.distinct_run_count, c.distinct_issue_count, c.highest_severity, c.status
		FROM invariant_proposals p
		JOIN finding_clusters c ON c.id = p.cluster_id
		WHERE p.id = $1
	`, proposalID).Scan(
		&proposal.ID, &proposal.ClusterID, &proposal.ScopeType, &proposal.ScopeKey, &proposal.Category, &proposal.Fingerprint, &proposal.ProposedText, &proposal.Rationale, &proposal.EvidenceSummary, &proposal.ProposalReason, &proposal.Status, &proposal.CreatedAt, &proposal.UpdatedAt,
		&cluster.ID, &cluster.ScopeType, &cluster.ScopeKey, &cluster.Category, &cluster.Fingerprint, &cluster.Title, &cluster.CanonicalSummary, &cluster.FirstSeenAt, &cluster.LastSeenAt, &cluster.OccurrenceCount, &cluster.DistinctRunCount, &cluster.DistinctIssueCount, &cluster.HighestSeverity, &cluster.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("load proposal: %w", err)
	}

	var invariant ActiveInvariant
	var sourceProposalID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO active_invariants (
			scope_type, scope_key, category, fingerprint, statement, severity, enforcement_mode, source_proposal_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (scope_type, scope_key, category, fingerprint) DO UPDATE SET
			statement = EXCLUDED.statement,
			severity = EXCLUDED.severity,
			enforcement_mode = EXCLUDED.enforcement_mode,
			source_proposal_id = EXCLUDED.source_proposal_id,
			updated_at = NOW()
		RETURNING id, scope_type, scope_key, category, fingerprint, statement, severity, enforcement_mode, source_proposal_id, created_at, updated_at
	`, proposal.ScopeType, proposal.ScopeKey, proposal.Category, proposal.Fingerprint, proposal.ProposedText, cluster.HighestSeverity, enforcementMode, proposal.ID).
		Scan(&invariant.ID, &invariant.ScopeType, &invariant.ScopeKey, &invariant.Category, &invariant.Fingerprint, &invariant.Statement, &invariant.Severity, &invariant.EnforcementMode, &sourceProposalID, &invariant.CreatedAt, &invariant.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("activate proposal: %w", err)
	}
	if sourceProposalID.Valid {
		invariant.SourceProposalID = &sourceProposalID.Int64
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO invariant_stage_stats (invariant_id, stage, occurrence_count, last_seen_at, rollback_count, block_count)
		SELECT $1, stage, occurrence_count, last_seen_at, rollback_count, block_count
		FROM cluster_stage_stats
		WHERE cluster_id = $2
		ON CONFLICT (invariant_id, stage) DO UPDATE SET
			occurrence_count = EXCLUDED.occurrence_count,
			last_seen_at = EXCLUDED.last_seen_at,
			rollback_count = EXCLUDED.rollback_count,
			block_count = EXCLUDED.block_count
	`, invariant.ID, cluster.ID); err != nil {
		return nil, fmt.Errorf("seed invariant stage stats: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE invariant_proposals SET status = 'approved', updated_at = NOW() WHERE id = $1
	`, proposal.ID); err != nil {
		return nil, fmt.Errorf("mark proposal approved: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &invariant, nil
}

func (s *PostgresStore) RankedInvariants(ctx context.Context, req RetrievalRequest) ([]RankedInvariant, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ai.id, ai.scope_type, ai.scope_key, ai.category, ai.fingerprint, ai.statement, ai.severity, ai.enforcement_mode, ai.source_proposal_id, ai.created_at, ai.updated_at,
		       iss.stage, iss.occurrence_count, iss.last_seen_at, iss.rollback_count, iss.block_count
		FROM active_invariants ai
		LEFT JOIN invariant_stage_stats iss
		  ON iss.invariant_id = ai.id AND iss.stage = $1
	`, req.Stage)
	if err != nil {
		return nil, fmt.Errorf("query ranked invariants: %w", err)
	}
	defer rows.Close()

	invariants := make([]ActiveInvariant, 0)
	stats := make([]InvariantStageStat, 0)
	for rows.Next() {
		var invariant ActiveInvariant
		var stat InvariantStageStat
		var sourceProposalID sql.NullInt64
		var stage sql.NullString
		var occurrenceCount sql.NullInt32
		var lastSeenAt sql.NullTime
		var rollbackCount sql.NullInt32
		var blockCount sql.NullInt32
		if err := rows.Scan(&invariant.ID, &invariant.ScopeType, &invariant.ScopeKey, &invariant.Category, &invariant.Fingerprint, &invariant.Statement, &invariant.Severity, &invariant.EnforcementMode, &sourceProposalID, &invariant.CreatedAt, &invariant.UpdatedAt,
			&stage, &occurrenceCount, &lastSeenAt, &rollbackCount, &blockCount); err != nil {
			return nil, err
		}
		if sourceProposalID.Valid {
			invariant.SourceProposalID = &sourceProposalID.Int64
		}
		invariants = append(invariants, invariant)
		if stage.Valid {
			stat = InvariantStageStat{
				InvariantID:     invariant.ID,
				Stage:           stage.String,
				OccurrenceCount: int(occurrenceCount.Int32),
				LastSeenAt:      lastSeenAt.Time,
				RollbackCount:   int(rollbackCount.Int32),
				BlockCount:      int(blockCount.Int32),
			}
			stats = append(stats, stat)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return RankForStage(time.Now().UTC(), invariants, statIndex(stats), req), nil
}

func (s *PostgresStore) readCluster(ctx context.Context, id int64) (FindingCluster, error) {
	var cluster FindingCluster
	err := s.db.QueryRowContext(ctx, `
		SELECT id, scope_type, scope_key, category, fingerprint, title, canonical_summary, first_seen_at, last_seen_at, occurrence_count, distinct_run_count, distinct_issue_count, highest_severity, status
		FROM finding_clusters
		WHERE id = $1
	`, id).Scan(
		&cluster.ID, &cluster.ScopeType, &cluster.ScopeKey, &cluster.Category, &cluster.Fingerprint, &cluster.Title,
		&cluster.CanonicalSummary, &cluster.FirstSeenAt, &cluster.LastSeenAt, &cluster.OccurrenceCount,
		&cluster.DistinctRunCount, &cluster.DistinctIssueCount, &cluster.HighestSeverity, &cluster.Status,
	)
	if err != nil {
		return FindingCluster{}, fmt.Errorf("read finding cluster by id: %w", err)
	}
	return cluster, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PostgresStore) readClusterTx(ctx context.Context, q queryRower, event FindingEvent) (FindingCluster, error) {
	var cluster FindingCluster
	query := `
		SELECT id, scope_type, scope_key, category, fingerprint, title, canonical_summary, first_seen_at, last_seen_at, occurrence_count, distinct_run_count, distinct_issue_count, highest_severity, status
		FROM finding_clusters
		WHERE scope_type = $1 AND scope_key = $2 AND category = $3 AND fingerprint = $4
	`
	err := q.QueryRowContext(ctx, query, event.ScopeType, event.ScopeKey, event.Category, event.Fingerprint).
		Scan(&cluster.ID, &cluster.ScopeType, &cluster.ScopeKey, &cluster.Category, &cluster.Fingerprint, &cluster.Title, &cluster.CanonicalSummary, &cluster.FirstSeenAt, &cluster.LastSeenAt, &cluster.OccurrenceCount, &cluster.DistinctRunCount, &cluster.DistinctIssueCount, &cluster.HighestSeverity, &cluster.Status)
	if err != nil {
		return FindingCluster{}, fmt.Errorf("read finding cluster: %w", err)
	}
	return cluster, nil
}

func (s *PostgresStore) readActiveInvariantTx(ctx context.Context, q queryRower, scopeType, scopeKey, category, fingerprint string) (*ActiveInvariant, error) {
	var invariant ActiveInvariant
	var sourceProposalID sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT id, scope_type, scope_key, category, fingerprint, statement, severity, enforcement_mode, source_proposal_id, created_at, updated_at
		FROM active_invariants
		WHERE scope_type = $1 AND scope_key = $2 AND category = $3 AND fingerprint = $4
	`, scopeType, scopeKey, category, fingerprint).
		Scan(&invariant.ID, &invariant.ScopeType, &invariant.ScopeKey, &invariant.Category, &invariant.Fingerprint, &invariant.Statement, &invariant.Severity, &invariant.EnforcementMode, &sourceProposalID, &invariant.CreatedAt, &invariant.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active invariant: %w", err)
	}
	if sourceProposalID.Valid {
		invariant.SourceProposalID = &sourceProposalID.Int64
	}
	return &invariant, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func highestSeveritySQL(newSeverity, existingSeverity string) string {
	return fmt.Sprintf(`CASE
		WHEN %s = 'critical' OR %s = 'critical' THEN 'critical'
		WHEN %s = 'high' OR %s = 'high' THEN 'high'
		WHEN %s = 'medium' OR %s = 'medium' THEN 'medium'
		WHEN %s = 'low' OR %s = 'low' THEN 'low'
		ELSE COALESCE(NULLIF(%s, ''), %s)
	END`, newSeverity, existingSeverity, newSeverity, existingSeverity, newSeverity, existingSeverity, newSeverity, existingSeverity, existingSeverity, newSeverity)
}
