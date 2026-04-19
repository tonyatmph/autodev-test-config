package ratchet

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	cluster         FindingCluster
	activeInvariant *ActiveInvariant
	openProposal    bool
	created         *InvariantProposal
	ranked          []RankedInvariant
}

func (f *fakeStore) Init(context.Context) error { return nil }
func (f *fakeStore) RecordFinding(context.Context, FindingEvent) (FindingCluster, *ActiveInvariant, error) {
	return f.cluster, f.activeInvariant, nil
}
func (f *fakeStore) HasOpenProposal(context.Context, int64) (bool, error) { return f.openProposal, nil }
func (f *fakeStore) CreateProposal(context.Context, FindingCluster, string) (*InvariantProposal, error) {
	f.created = &InvariantProposal{ID: 1, ProposalReason: "frequency_threshold"}
	return f.created, nil
}
func (f *fakeStore) ActivateProposal(context.Context, int64, string) (*ActiveInvariant, error) {
	return &ActiveInvariant{ID: 1}, nil
}
func (f *fakeStore) RankedInvariants(context.Context, RetrievalRequest) ([]RankedInvariant, error) {
	return f.ranked, nil
}

func TestIngestFindingCreatesProposalWhenThresholdCrossed(t *testing.T) {
	store := &fakeStore{
		cluster: FindingCluster{
			ID:               10,
			OccurrenceCount:  3,
			DistinctRunCount: 3,
			HighestSeverity:  "medium",
			Title:            "Avoid plaintext secrets in manifests",
		},
	}
	service := NewService(store)

	result, err := service.IngestFinding(context.Background(), FindingEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal == nil {
		t.Fatal("expected proposal to be created")
	}
}

func TestRankForStagePrioritizesStageRelevantInvariant(t *testing.T) {
	now := time.Now().UTC()
	invariants := []ActiveInvariant{
		{ID: 1, ScopeType: "global", Statement: "Invariant A", Severity: "medium", EnforcementMode: "warn"},
		{ID: 2, ScopeType: "global", Statement: "Invariant B", Severity: "low", EnforcementMode: "inform"},
	}
	stats := map[int64]InvariantStageStat{
		1: {InvariantID: 1, Stage: "implement", OccurrenceCount: 5, LastSeenAt: now, RollbackCount: 1},
		2: {InvariantID: 2, Stage: "implement", OccurrenceCount: 1, LastSeenAt: now.Add(-10 * 24 * time.Hour)},
	}
	ranked := RankForStage(now, invariants, stats, RetrievalRequest{Stage: "implement", Limit: 5})
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked invariants, got %d", len(ranked))
	}
	if ranked[0].ID != 1 {
		t.Fatalf("expected invariant 1 to rank first, got %d", ranked[0].ID)
	}
}
