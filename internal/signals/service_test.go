package signals

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	inserted []PipelineEvent
	recent   []PipelineEvent
	count    int
	upserted []OperationalSignal
	listed   []OperationalSignal
}

func (f *fakeStore) Init(context.Context) error { return nil }
func (f *fakeStore) InsertEvent(_ context.Context, event PipelineEvent) (PipelineEvent, error) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	f.inserted = append(f.inserted, event)
	return event, nil
}
func (f *fakeStore) RecentEvents(context.Context, string, string, string, string, time.Time, int, string) ([]PipelineEvent, error) {
	return f.recent, nil
}
func (f *fakeStore) CountEvents(context.Context, string, string, string, time.Time) (int, error) {
	return f.count, nil
}
func (f *fakeStore) UpsertSignal(_ context.Context, signal OperationalSignal) (OperationalSignal, error) {
	signal.ID = int64(len(f.upserted) + 1)
	signal.EventCount = 1
	f.upserted = append(f.upserted, signal)
	return signal, nil
}
func (f *fakeStore) ListSignals(context.Context, ListRequest) ([]OperationalSignal, error) {
	return f.listed, nil
}

func TestRecordPipelineEventCreatesFailureSignal(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)

	signals, err := service.RecordPipelineEvent(context.Background(), PipelineEvent{
		Kind:      "stage_completed",
		IssueID:   "issue-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Stage:     "implement",
		RepoScope: "mph-tech/example",
		Status:    "failed",
		Summary:   "implement failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Category != "pipeline_failure" {
		t.Fatalf("unexpected signals: %+v", signals)
	}
}

func TestRecordPipelineEventCreatesTimingRegressionSignal(t *testing.T) {
	store := &fakeStore{
		recent: []PipelineEvent{
			{DurationMS: 1000, CostUSD: 0.01},
			{DurationMS: 1100, CostUSD: 0.01},
			{DurationMS: 900, CostUSD: 0.009},
		},
	}
	service := NewService(store)

	signals, err := service.RecordPipelineEvent(context.Background(), PipelineEvent{
		Kind:       "stage_completed",
		IssueID:    "issue-1",
		RunID:      "run-1",
		AttemptID:  "attempt-2",
		Stage:      "security",
		RepoScope:  "mph-tech/example",
		Status:     "succeeded",
		Summary:    "security passed",
		DurationMS: 2600,
		CostUSD:    0.012,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Category != "pipeline_timing_regression" {
		t.Fatalf("unexpected signals: %+v", signals)
	}
}

func TestRecordPipelineEventCreatesContentionHotspotSignal(t *testing.T) {
	store := &fakeStore{count: 3}
	service := NewService(store)

	signals, err := service.RecordPipelineEvent(context.Background(), PipelineEvent{
		Kind:      "lock_contention",
		IssueID:   "issue-1",
		RunID:     "run-1",
		AttemptID: "attempt-3",
		Stage:     "promote_dev",
		RepoScope: "mph-tech/example",
		Summary:   "gitops repo lock busy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Category != "pipeline_contention_hotspot" {
		t.Fatalf("unexpected signals: %+v", signals)
	}
}
