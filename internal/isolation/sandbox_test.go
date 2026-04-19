package isolation

import (
	"os"
	"path/filepath"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestSandboxDeniesWritesOutsideOwnedPaths(t *testing.T) {
	rootDir := t.TempDir()
	dataDir := t.TempDir()
	repoDir := filepath.Join(rootDir, "service")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	run := model.RunRequest{
		ID: "run-1",
		WorkOrder: model.WorkOrder{
			Delivery: model.DeliveryTarget{
				SelectedComponents: []string{"api"},
				Components: model.DeliveryComponents{
					"api": {
						Name: "api",
						Repo: model.RepoTarget{ProjectPath: "service"},
						Ownership: []model.PathOwnershipRule{
							{Identity: model.ExecutionIdentityAgent, Paths: []string{"src/**"}, Mutable: true},
							{Identity: model.ExecutionIdentityGenerator, Paths: []string{"generated/**"}, Mutable: true},
						},
					},
				},
			},
		},
	}

	agentSandbox := New(rootDir, dataDir, nil, model.StageSpec{
		Name:            "implement",
		Operation:       "orchestrate",
		OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityAgent,
			WriteAs: model.ExecutionIdentityAgent,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceComponents)},
				RepoControl: []string{string(model.StageSurfaceComponents)},
			},
		},
	}, run)
	if err := agentSandbox.RequireWrite(filepath.Join(repoDir, "src", "autodev", "note.txt")); err != nil {
		t.Fatalf("expected src write allowed: %v", err)
	}
	if err := agentSandbox.RequireWrite(filepath.Join(repoDir, "generated", "autodev", "note.json")); err == nil {
		t.Fatal("expected generated write to be denied for agent identity")
	}

	generatorSandbox := New(rootDir, dataDir, nil, model.StageSpec{
		Name:            "generate",
		Operation:       "orchestrate",
		OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityGenerator,
			WriteAs: model.ExecutionIdentityGenerator,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceComponents), string(model.StageSurfaceJournal)},
				RepoControl: []string{string(model.StageSurfaceComponents), string(model.StageSurfaceJournal)},
			},
			Materialize: []string{string(model.StageSurfaceComponents), string(model.StageSurfaceJournal)},
		},
	}, run)
	if err := generatorSandbox.RequireWrite(filepath.Join(repoDir, "generated", "autodev", "note.json")); err != nil {
		t.Fatalf("expected generated write allowed: %v", err)
	}
}

func TestGovernedSandboxAllowsGitOpsPathOnly(t *testing.T) {
	rootDir := t.TempDir()
	dataDir := t.TempDir()
	gitopsDir := filepath.Join(rootDir, "gitops")
	if err := os.MkdirAll(gitopsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	run := model.RunRequest{
		ID: "run-1",
		WorkOrder: model.WorkOrder{
			Delivery: model.DeliveryTarget{
				Environments: model.PromotionTargets{
					Dev: model.EnvironmentTarget{
						Name: "dev",
						GitOpsRepo: model.GitOpsTarget{
							ProjectPath: "gitops",
							Path:        "clusters/dev/app",
						},
					},
				},
			},
		},
	}

	sandbox := New(rootDir, dataDir, nil, model.StageSpec{
		Name:            "promote_dev",
		Operation:       "orchestrate",
		OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
		Runtime:         model.StageRuntime{Environment: "dev"},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityGoverned,
			WriteAs: model.ExecutionIdentityGoverned,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceGitOps)},
				RepoControl: []string{string(model.StageSurfaceGitOps)},
			},
			Materialize: []string{string(model.StageSurfaceGitOps)},
		},
	}, run)
	if err := sandbox.RequireWrite(filepath.Join(gitopsDir, "clusters", "dev", "app", "app.yaml")); err != nil {
		t.Fatalf("expected gitops path write allowed: %v", err)
	}
	if err := sandbox.RequireWrite(filepath.Join(rootDir, "other-repo", "README.md")); err == nil {
		t.Fatal("expected write outside the governed gitops repo to be denied")
	}
}
