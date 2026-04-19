package signals

import (
	"context"
	"time"
)

type PipelineEvent struct {
	ID           int64          `json:"id"`
	Kind         string         `json:"kind"`
	RunID        string         `json:"run_id"`
	IssueID      string         `json:"issue_id"`
	AttemptID    string         `json:"attempt_id"`
	Stage        string         `json:"stage"`
	WorkerID     string         `json:"worker_id,omitempty"`
	RepoScope    string         `json:"repo_scope"`
	Environment  string         `json:"environment,omitempty"`
	ServiceScope string         `json:"service_scope,omitempty"`
	Status       string         `json:"status,omitempty"`
	Severity     string         `json:"severity,omitempty"`
	Summary      string         `json:"summary"`
	DurationMS   int64          `json:"duration_ms,omitempty"`
	CostUSD      float64        `json:"cost_usd,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type OperationalSignal struct {
	ID           int64     `json:"id"`
	Category     string    `json:"category"`
	Fingerprint  string    `json:"fingerprint"`
	RunID        string    `json:"run_id"`
	IssueID      string    `json:"issue_id"`
	Stage        string    `json:"stage"`
	RepoScope    string    `json:"repo_scope"`
	Environment  string    `json:"environment,omitempty"`
	ServiceScope string    `json:"service_scope,omitempty"`
	Severity     string    `json:"severity"`
	Status       string    `json:"status"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	EventCount   int       `json:"event_count"`
	LastEventAt  time.Time `json:"last_event_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ListRequest struct {
	RepoScope string `json:"repo_scope"`
	Stage     string `json:"stage"`
	Status    string `json:"status"`
	Limit     int    `json:"limit"`
}

type Emitter interface {
	RecordPipelineEvent(ctx context.Context, event PipelineEvent) ([]OperationalSignal, error)
}
