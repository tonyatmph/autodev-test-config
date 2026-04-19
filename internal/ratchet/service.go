package ratchet

import (
	"context"
)

type Store interface {
	Init(ctx context.Context) error
	RecordFinding(ctx context.Context, event FindingEvent) (FindingCluster, *ActiveInvariant, error)
	HasOpenProposal(ctx context.Context, clusterID int64) (bool, error)
	CreateProposal(ctx context.Context, cluster FindingCluster, reason string) (*InvariantProposal, error)
	ActivateProposal(ctx context.Context, proposalID int64, enforcementMode string) (*ActiveInvariant, error)
	RankedInvariants(ctx context.Context, req RetrievalRequest) ([]RankedInvariant, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Init(ctx context.Context) error {
	return s.store.Init(ctx)
}

func (s *Service) IngestFinding(ctx context.Context, event FindingEvent) (IngestResult, error) {
	cluster, activeInvariant, err := s.store.RecordFinding(ctx, event)
	if err != nil {
		return IngestResult{}, err
	}
	if activeInvariant != nil {
		return IngestResult{Cluster: cluster}, nil
	}
	propose, reason := shouldPropose(cluster)
	if !propose {
		return IngestResult{Cluster: cluster}, nil
	}
	exists, err := s.store.HasOpenProposal(ctx, cluster.ID)
	if err != nil {
		return IngestResult{}, err
	}
	if exists {
		return IngestResult{Cluster: cluster}, nil
	}
	proposal, err := s.store.CreateProposal(ctx, cluster, reason)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Cluster: cluster, Proposal: proposal}, nil
}

func (s *Service) ActivateProposal(ctx context.Context, proposalID int64, enforcementMode string) (*ActiveInvariant, error) {
	return s.store.ActivateProposal(ctx, proposalID, enforcementMode)
}

func (s *Service) RankedInvariants(ctx context.Context, req RetrievalRequest) ([]RankedInvariant, error) {
	return s.store.RankedInvariants(ctx, req)
}

type NoopStore struct{}

func (NoopStore) Init(context.Context) error { return nil }

func (NoopStore) RecordFinding(context.Context, FindingEvent) (FindingCluster, *ActiveInvariant, error) {
	return FindingCluster{}, nil, nil
}

func (NoopStore) HasOpenProposal(context.Context, int64) (bool, error) { return false, nil }

func (NoopStore) CreateProposal(context.Context, FindingCluster, string) (*InvariantProposal, error) {
	return nil, nil
}

func (NoopStore) ActivateProposal(context.Context, int64, string) (*ActiveInvariant, error) {
	return nil, nil
}

func (NoopStore) RankedInvariants(context.Context, RetrievalRequest) ([]RankedInvariant, error) {
	return nil, nil
}
