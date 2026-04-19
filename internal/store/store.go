package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Store struct {
	root string
	mu   sync.Mutex
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Load() (model.PersistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *Store) Save(update func(*model.PersistedState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	if err := update(&state); err != nil {
		return err
	}
	return s.saveUnlocked(state)
}

func (s *Store) path() string {
	return filepath.Join(s.root, "state.json")
}

func (s *Store) loadUnlocked() (model.PersistedState, error) {
	state := model.PersistedState{
		Issues:   map[string]model.TrackedIssue{},
		Runs:     map[string]model.RunRequest{},
		Attempts: map[string]model.StageAttempt{},
		Counters: map[string]int{},
	}

	path := s.path()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read state: %w", err)
	}
	if err := contracts.Unmarshal(data, contracts.PersistedStateSchema, path, &state); err != nil {
		return state, fmt.Errorf("decode state: %w", err)
	}

	if state.Issues == nil {
		state.Issues = map[string]model.TrackedIssue{}
	}
	if state.Runs == nil {
		state.Runs = map[string]model.RunRequest{}
	}
	if state.Attempts == nil {
		state.Attempts = map[string]model.StageAttempt{}
	}
	if state.Counters == nil {
		state.Counters = map[string]int{}
	}
	return state, nil
}

func (s *Store) saveUnlocked(state model.PersistedState) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := contracts.WriteFile(s.path(), contracts.PersistedStateSchema, state); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func NextID(state *model.PersistedState, prefix string) string {
	state.Counters[prefix]++
	return fmt.Sprintf("%s-%05d", prefix, state.Counters[prefix])
}

func AttemptsByRun(state model.PersistedState, runID string) []model.StageAttempt {
	out := make([]model.StageAttempt, 0)
	for _, attempt := range state.Attempts {
		if attempt.RunID == runID {
			out = append(out, attempt)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stage == out[j].Stage {
			return out[i].Attempt < out[j].Attempt
		}
		return out[i].Stage < out[j].Stage
	})
	return out
}

func AttemptsByStage(state model.PersistedState, runID string) map[string]model.StageAttempt {
	out := make(map[string]model.StageAttempt)
	for _, attempt := range state.Attempts {
		if attempt.RunID != runID {
			continue
		}
		current, ok := out[attempt.Stage]
		if !ok || attempt.Attempt > current.Attempt {
			out[attempt.Stage] = attempt
		}
	}
	return out
}

func RunningCount(state model.PersistedState, stage string) int {
	count := 0
	now := time.Now().UTC()
	for _, attempt := range state.Attempts {
		if attempt.Stage != stage || attempt.Status != model.AttemptStatusRunning {
			continue
		}
		if attempt.LeaseExpiresAt != nil && attempt.LeaseExpiresAt.Before(now) {
			continue
		}
		count++
	}
	return count
}
