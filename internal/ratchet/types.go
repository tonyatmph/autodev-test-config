package ratchet

import "time"

type FindingEvent struct {
	ID            int64     `json:"id"`
	RunID         string    `json:"run_id"`
	IssueID       string    `json:"issue_id"`
	Stage         string    `json:"stage"`
	RepoScope     string    `json:"repo_scope"`
	Environment   string    `json:"environment"`
	ServiceScope  string    `json:"service_scope"`
	ScopeType     string    `json:"scope_type"`
	ScopeKey      string    `json:"scope_key"`
	Category      string    `json:"category"`
	Fingerprint   string    `json:"fingerprint"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	Severity      string    `json:"severity"`
	OutcomeImpact string    `json:"outcome_impact"`
	EvidenceURI   string    `json:"evidence_uri"`
	CreatedAt     time.Time `json:"created_at"`
}

type FindingCluster struct {
	ID                 int64     `json:"id"`
	ScopeType          string    `json:"scope_type"`
	ScopeKey           string    `json:"scope_key"`
	Category           string    `json:"category"`
	Fingerprint        string    `json:"fingerprint"`
	Title              string    `json:"title"`
	CanonicalSummary   string    `json:"canonical_summary"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	OccurrenceCount    int       `json:"occurrence_count"`
	DistinctRunCount   int       `json:"distinct_run_count"`
	DistinctIssueCount int       `json:"distinct_issue_count"`
	HighestSeverity    string    `json:"highest_severity"`
	Status             string    `json:"status"`
}

type ClusterStageStat struct {
	ClusterID       int64     `json:"cluster_id"`
	Stage           string    `json:"stage"`
	OccurrenceCount int       `json:"occurrence_count"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	RollbackCount   int       `json:"rollback_count"`
	BlockCount      int       `json:"block_count"`
}

type InvariantProposal struct {
	ID              int64     `json:"id"`
	ClusterID       int64     `json:"cluster_id"`
	ScopeType       string    `json:"scope_type"`
	ScopeKey        string    `json:"scope_key"`
	Category        string    `json:"category"`
	Fingerprint     string    `json:"fingerprint"`
	ProposedText    string    `json:"proposed_text"`
	Rationale       string    `json:"rationale"`
	EvidenceSummary string    `json:"evidence_summary"`
	ProposalReason  string    `json:"proposal_reason"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ActiveInvariant struct {
	ID               int64     `json:"id"`
	ScopeType        string    `json:"scope_type"`
	ScopeKey         string    `json:"scope_key"`
	Category         string    `json:"category"`
	Fingerprint      string    `json:"fingerprint"`
	Statement        string    `json:"statement"`
	Severity         string    `json:"severity"`
	EnforcementMode  string    `json:"enforcement_mode"`
	SourceProposalID *int64    `json:"source_proposal_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type InvariantStageStat struct {
	InvariantID     int64     `json:"invariant_id"`
	Stage           string    `json:"stage"`
	OccurrenceCount int       `json:"occurrence_count"`
	LastSeenAt      time.Time `json:"last_seen_at"`
	RollbackCount   int       `json:"rollback_count"`
	BlockCount      int       `json:"block_count"`
}

type RankedInvariant struct {
	ActiveInvariant
	StageStat *InvariantStageStat `json:"stage_stat,omitempty"`
	Score     float64             `json:"score"`
}

type RetrievalRequest struct {
	Stage        string `json:"stage"`
	RepoScope    string `json:"repo_scope"`
	Environment  string `json:"environment"`
	ServiceScope string `json:"service_scope"`
	Limit        int    `json:"limit"`
}

type IngestResult struct {
	Cluster  FindingCluster     `json:"cluster"`
	Proposal *InvariantProposal `json:"proposal,omitempty"`
}
