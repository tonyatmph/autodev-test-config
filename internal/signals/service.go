package signals

import (
	"context"
	"fmt"
	"time"
)

type Store interface {
	Init(ctx context.Context) error
	InsertEvent(ctx context.Context, event PipelineEvent) (PipelineEvent, error)
	RecentEvents(ctx context.Context, kind, stage, repoScope, status string, since time.Time, limit int, excludeAttemptID string) ([]PipelineEvent, error)
	CountEvents(ctx context.Context, kind, stage, repoScope string, since time.Time) (int, error)
	UpsertSignal(ctx context.Context, signal OperationalSignal) (OperationalSignal, error)
	ListSignals(ctx context.Context, req ListRequest) ([]OperationalSignal, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Init(ctx context.Context) error {
	return s.store.Init(ctx)
}

func (s *Service) RecordPipelineEvent(ctx context.Context, event PipelineEvent) ([]OperationalSignal, error) {
	stored, err := s.store.InsertEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	candidates, err := s.synthesize(ctx, stored)
	if err != nil {
		return nil, err
	}
	out := make([]OperationalSignal, 0, len(candidates))
	for _, candidate := range candidates {
		signal, err := s.store.UpsertSignal(ctx, candidate)
		if err != nil {
			return nil, err
		}
		out = append(out, signal)
	}
	return out, nil
}

func (s *Service) ListSignals(ctx context.Context, req ListRequest) ([]OperationalSignal, error) {
	return s.store.ListSignals(ctx, req)
}

func (s *Service) synthesize(ctx context.Context, event PipelineEvent) ([]OperationalSignal, error) {
	now := s.now()
	signals := make([]OperationalSignal, 0, 2)
	switch event.Kind {
	case "stage_completed":
		switch event.Status {
		case "failed":
			severity := event.Severity
			if severity == "" {
				severity = "medium"
			}
			signals = append(signals, makeSignal(event, "pipeline_failure", severity, fmt.Sprintf("Stage %s failures need triage", event.Stage), event.Summary))
		case "blocked":
			signals = append(signals, makeSignal(event, "pipeline_blocked", "medium", fmt.Sprintf("Stage %s is blocked", event.Stage), event.Summary))
		case "succeeded":
			recent, err := s.store.RecentEvents(ctx, "stage_completed", event.Stage, event.RepoScope, "succeeded", now.Add(-7*24*time.Hour), 20, event.AttemptID)
			if err != nil {
				return nil, err
			}
			if len(recent) >= 3 {
				avgDuration, avgCost := averages(recent)
				if avgDuration > 0 && event.DurationMS > avgDuration*2 && event.DurationMS-avgDuration > 500 {
					signals = append(signals, makeSignal(event, "pipeline_timing_regression", "medium",
						fmt.Sprintf("Stage %s timing regression detected", event.Stage),
						fmt.Sprintf("Duration rose to %dms from a recent baseline of %dms.", event.DurationMS, avgDuration)))
				}
				if avgCost > 0 && event.CostUSD > avgCost*2 && event.CostUSD-avgCost > 0.005 {
					signals = append(signals, makeSignal(event, "pipeline_cost_anomaly", "medium",
						fmt.Sprintf("Stage %s cost anomaly detected", event.Stage),
						fmt.Sprintf("Cost rose to $%.6f from a recent baseline of $%.6f.", event.CostUSD, avgCost)))
				}
			}
		}
	case "lock_contention":
		count, err := s.store.CountEvents(ctx, "lock_contention", event.Stage, event.RepoScope, now.Add(-time.Hour))
		if err != nil {
			return nil, err
		}
		if count >= 3 {
			signals = append(signals, makeSignal(event, "pipeline_contention_hotspot", "medium",
				fmt.Sprintf("Stage %s lock contention hotspot", event.Stage),
				fmt.Sprintf("Lock contention occurred %d times in the last hour for %s.", count, event.RepoScope)))
		}
	case "stale_attempt_recovered":
		signals = append(signals, makeSignal(event, "pipeline_stale_recovery", "low",
			fmt.Sprintf("Recovered stale %s attempt", event.Stage), event.Summary))
	}
	return signals, nil
}

func makeSignal(event PipelineEvent, category, severity, title, summary string) OperationalSignal {
	scope := event.RepoScope
	if scope == "" {
		scope = "global"
	}
	return OperationalSignal{
		Category:     category,
		Fingerprint:  fmt.Sprintf("%s|%s|%s|%s", category, scope, event.Stage, event.Environment),
		RunID:        event.RunID,
		IssueID:      event.IssueID,
		Stage:        event.Stage,
		RepoScope:    event.RepoScope,
		Environment:  event.Environment,
		ServiceScope: event.ServiceScope,
		Severity:     severity,
		Status:       "open",
		Title:        title,
		Summary:      summary,
		LastEventAt:  event.CreatedAt,
	}
}

func averages(events []PipelineEvent) (int64, float64) {
	if len(events) == 0 {
		return 0, 0
	}
	var totalDuration int64
	var totalCost float64
	for _, event := range events {
		totalDuration += event.DurationMS
		totalCost += event.CostUSD
	}
	return totalDuration / int64(len(events)), totalCost / float64(len(events))
}

type NoopStore struct{}

func (NoopStore) Init(context.Context) error { return nil }

func (NoopStore) InsertEvent(_ context.Context, event PipelineEvent) (PipelineEvent, error) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return event, nil
}

func (NoopStore) RecentEvents(context.Context, string, string, string, string, time.Time, int, string) ([]PipelineEvent, error) {
	return nil, nil
}

func (NoopStore) CountEvents(context.Context, string, string, string, time.Time) (int, error) {
	return 0, nil
}

func (NoopStore) UpsertSignal(_ context.Context, signal OperationalSignal) (OperationalSignal, error) {
	if signal.CreatedAt.IsZero() {
		now := time.Now().UTC()
		signal.CreatedAt = now
		signal.UpdatedAt = now
	}
	signal.EventCount = 1
	return signal, nil
}

func (NoopStore) ListSignals(context.Context, ListRequest) ([]OperationalSignal, error) {
	return nil, nil
}
