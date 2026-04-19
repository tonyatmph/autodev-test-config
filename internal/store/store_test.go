package store

import (
	"os"
	"path/filepath"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestStorePersistsStateThroughContractHelper(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.Save(func(state *model.PersistedState) error {
		state.Counters["run"] = 7
		state.Runs["run-00007"] = model.RunRequest{ID: "run-00007", IssueID: "issue-1", Status: model.RunStatusPending}
		return nil
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.Counters["run"] != 7 {
		t.Fatalf("expected counter to persist, got %+v", loaded.Counters)
	}
	if loaded.Runs["run-00007"].ID != "run-00007" {
		t.Fatalf("expected run to persist, got %+v", loaded.Runs)
	}
}

func TestStoreRejectsMalformedStateSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	if err := os.WriteFile(path, []byte(`{"issues":[],"runs":{},"attempts":{},"counters":{}}`), 0o644); err != nil {
		t.Fatalf("write malformed state: %v", err)
	}
	if _, err := New(root).Load(); err == nil {
		t.Fatal("expected malformed state schema to fail")
	}
}
