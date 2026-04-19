package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/configsource"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/locks"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/pipeline"
	"g7.mph.tech/mph-tech/autodev/internal/signals"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
	"g7.mph.tech/mph-tech/autodev/internal/store"
	"g7.mph.tech/mph-tech/autodev/internal/workorder"
)

func TestReconcileDoesNotCreateRunWithoutMaterialization(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{
		ID:               "issue-1",
		Labels:           []string{model.IssueLabelRequested},
		PipelineTemplate: "default-v1",
		Target: model.DeliveryTarget{
			ApplicationRepo: model.RepoTarget{
				ProjectPath:         "group/repo",
				DefaultBranch:       "main",
				WorkingBranchPrefix: "autodev",
			},
			Environments: model.PromotionTargets{
				Local: model.EnvironmentTarget{
					Name: "local",
					GitOpsRepo: model.GitOpsTarget{
						ProjectPath:     "group/repo-gitops",
						Environment:     "local",
						Path:            "clusters/local/repo",
						PromotionBranch: "main",
						Cluster:         "kind-local",
					},
				},
				Dev: model.EnvironmentTarget{
					Name: "dev",
					GitOpsRepo: model.GitOpsTarget{
						ProjectPath:     "group/repo-gitops",
						Environment:     "dev",
						Path:            "clusters/dev/repo",
						PromotionBranch: "main",
						Cluster:         "dev-us-east-1",
					},
				},
				Prod: model.EnvironmentTarget{
					Name: "prod",
					GitOpsRepo: model.GitOpsTarget{
						ProjectPath:     "group/repo-gitops",
						Environment:     "prod",
						Path:            "clusters/prod/repo",
						PromotionBranch: "main",
						Cluster:         "prod-us-east-1",
					},
					ApprovalRequired: true,
				},
			},
			Release: model.ReleaseDefinition{
				Application: model.ApplicationRelease{
					ArtifactName: "repo",
					ImageRepo:    "registry.mph.tech/repo",
				},
				Infrastructure: model.InfrastructureChange{
					Ref:           "infra/repo@v1",
					GitOpsOnly:    true,
					TerraformRoot: "terraform/repo",
				},
				Database: model.DatabaseChange{
					BundleRef:        "db/repo/v1",
					Compatibility:    "expand-contract",
					GitOpsManagedJob: true,
				},
			},
		},
		Approval: model.ApprovalGate{Label: "delivery/approved"},
	})

	if err := service.EnqueueFromGitLab(); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Runs) != 0 {
		t.Fatalf("expected no runs before materialization, got %d", len(state.Runs))
	}
	if len(state.Attempts) != 0 {
		t.Fatalf("expected no attempts before materialization, got %d", len(state.Attempts))
	}
}

func TestEnsureRunForIssueAndPersistPipelineArtifacts(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-materialize",
		Labels: []string{model.IssueLabelRequested},
		WorkOrder: model.WorkOrder{
			ID: "wo-materialize",
			Delivery: model.DeliveryTarget{
				Name: "demo",
			},
		},
	})
	if err := service.EnqueueFromGitLab(); err != nil {
		t.Fatal(err)
	}
	run, err := service.EnsureRunForIssue("issue-materialize")
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]any{
		"pipeline_intent":         validPipelineIntentForTest(),
		"policy_evaluation":       validPolicyEvaluationForTest(),
		"pipeline_build_plan":     validPipelineBuildPlanForTest(),
		"pipeline_execution_plan": validPipelineExecutionPlanForTest([]map[string]any{{"name": "intake"}}),
	}
	run, err = service.PersistPipelineArtifacts(run.ID, outputs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := service.pipelinePlanForRun(run); !ok {
		t.Fatal("expected persisted pipeline execution plan")
	}
	for _, rel := range []string{
		"work-orders/wo-materialize/runs/" + run.ID + "/pipeline/pipeline_intent.json",
		"work-orders/wo-materialize/runs/" + run.ID + "/pipeline/policy_evaluation.json",
		"work-orders/wo-materialize/runs/" + run.ID + "/pipeline/pipeline_build_plan.json",
		"work-orders/wo-materialize/runs/" + run.ID + "/pipeline/pipeline_execution_plan.json",
	} {
		if _, err := os.Stat(filepath.Join(workOrderDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
}

func TestEnsureRunForIssueCreatesNewRunAfterTerminalState(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-rerun",
		Labels: []string{model.IssueLabelRequested},
		WorkOrder: model.WorkOrder{
			ID: "wo-rerun",
			Delivery: model.DeliveryTarget{
				Name: "demo",
			},
		},
	})
	if err := service.EnqueueFromGitLab(); err != nil {
		t.Fatal(err)
	}
	first, err := service.EnsureRunForIssue("issue-rerun")
	if err != nil {
		t.Fatal(err)
	}
	err = service.store.Save(func(state *model.PersistedState) error {
		run := state.Runs[first.ID]
		run.Status = model.RunStatusFailed
		state.Runs[first.ID] = run
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.EnsureRunForIssue("issue-rerun")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatalf("expected new run after terminal state, got same run %s", second.ID)
	}
}

func TestRecoverStuckRunsReturnsExpiredAttemptsToPending(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC().Add(-2 * time.Minute)
		state.Runs["run-1"] = model.RunRequest{ID: "run-1", IssueID: "issue-1"}
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{
				ID:     "issue-1",
				Labels: []string{model.IssueLabelActive},
			},
		}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:             "attempt-1",
			RunID:          "run-1",
			Stage:          "plan",
			Attempt:        1,
			Status:         model.AttemptStatusRunning,
			LeaseExpiresAt: &now,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.RecoverStuckRuns(30 * time.Second); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Attempts["attempt-1"].Status != model.AttemptStatusPending {
		t.Fatalf("expected recovered attempt to be pending, got %s", state.Attempts["attempt-1"].Status)
	}
}

func TestCompleteAggregatesRunStats(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{ID: "issue-1", Labels: []string{model.IssueLabelActive}},
		}
		state.Runs["run-1"] = model.RunRequest{ID: "run-1", IssueID: "issue-1", Status: model.RunStatusActive, CreatedAt: now, UpdatedAt: now}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:      "attempt-1",
			RunID:   "run-1",
			Stage:   "plan",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	result := model.StageResult{
		Status:  model.AttemptStatusSucceeded,
		Summary: "planned",
		Stats: model.AttemptStats{
			Stage:      "plan",
			DurationMS: 1250,
			Substages: []model.SubstageTiming{
				{Name: "prepare_workspace", DurationMS: 150},
				{Name: "load_invariants", DurationMS: 90},
			},
			Cost:  model.CostBreakdown{Currency: "USD", TotalUSD: 0.0185},
			Usage: model.UsageMetrics{Model: "autodev-stage-v1"},
		},
	}
	artifacts := []model.ArtifactRef{{Name: "result"}, {Name: "stats"}}
	if err := service.Complete("attempt-1", result, artifacts); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	run := state.Runs["run-1"]
	if run.Stats.TotalCostUSD != 0.0185 {
		t.Fatalf("expected total cost 0.0185, got %f", run.Stats.TotalCostUSD)
	}
	if run.Stats.TotalDurationMS != 1250 {
		t.Fatalf("expected total duration 1250, got %d", run.Stats.TotalDurationMS)
	}
	if run.Stats.ArtifactCount != 2 {
		t.Fatalf("expected artifact count 2, got %d", run.Stats.ArtifactCount)
	}
	if len(run.Stats.Stages) != 1 || run.Stats.Stages[0].Stage != "plan" {
		t.Fatalf("unexpected stage totals: %+v", run.Stats.Stages)
	}
	if len(run.Stats.Stages[0].Substages) != 2 {
		t.Fatalf("expected 2 substage totals, got %+v", run.Stats.Stages[0].Substages)
	}
}

func TestCompleteStoresJournalMetadata(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{ID: "issue-1", Labels: []string{model.IssueLabelActive}},
		}
		state.Runs["run-1"] = model.RunRequest{ID: "run-1", IssueID: "issue-1", Status: model.RunStatusActive, CreatedAt: now, UpdatedAt: now}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:      "attempt-1",
			RunID:   "run-1",
			Stage:   "implement",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	outputs := map[string]any{
		"implementation_bundle": map[string]any{
			"components": []any{
				map[string]any{
					"name":         "api",
					"branch_name":  "autodev/run-1/api",
					"project_path": "api",
					"repo_status":  "resolved",
					"mutation_outcome": map[string]any{
						"mutated":    true,
						"commit_sha": "sha256:deadbeef",
					},
				},
			},
		},
	}
	outputBytes, _ := json.Marshal(outputs)
	result := model.StageResult{
		Status:  model.AttemptStatusSucceeded,
		Summary: "implemented",
		Outputs: outputBytes,
		Stats: model.AttemptStats{
			Stage:      "implement",
			DurationMS: 600,
			Cost:       model.CostBreakdown{Currency: "USD", TotalUSD: 0.0025},
			Usage:      model.UsageMetrics{Model: "autodev-stage-v1"},
		},
	}
	artifacts := []model.ArtifactRef{{Name: "result"}}
	if err := service.Complete("attempt-1", result, artifacts); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	attempt := state.Attempts["attempt-1"]
	entry, ok := attempt.Metadata["journal_entry"].(map[string]any)
	if !ok {
		t.Fatalf("expected journal_entry metadata, got %v", attempt.Metadata)
	}
	if entry["stage"] != "implement" {
		t.Fatalf("unexpected journal stage: %v", entry["stage"])
	}
	if commits, ok := entry["component_commits"].([]map[string]any); ok {
		if len(commits) != 1 {
			t.Fatalf("expected one component commit, got %v", commits)
		}
		if commits[0]["commit_sha"] != "sha256:deadbeef" {
			t.Fatalf("unexpected commit sha: %v", commits[0]["commit_sha"])
		}
	} else if commits, ok := entry["component_commits"].([]any); ok {
		if len(commits) != 1 {
			t.Fatalf("expected one component commit entry, got %v", commits)
		}
	} else {
		t.Fatalf("no component commits found in journal entry")
	}

	run := state.Runs["run-1"]
	history, ok := run.Metadata["journal_history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected journal_history with one entry, got %v", run.Metadata["journal_history"])
	}
	if last, _ := run.Metadata["last_journal"].(map[string]any); last == nil {
		t.Fatalf("expected last_journal metadata")
	}
	stageReport, ok := attempt.Metadata["stage_report"].(map[string]any)
	if !ok {
		t.Fatalf("expected stage_report metadata, got %v", attempt.Metadata)
	}
	if _, ok := stageReport["report"].(string); !ok {
		t.Fatalf("expected report path in stage_report metadata, got %v", stageReport)
	}
	if _, ok := run.Metadata["run_index"].(string); !ok {
		t.Fatalf("expected run_index metadata, got %v", run.Metadata)
	}
}

func TestHeartbeatPersistsRealtimeStageState(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{ID: "issue-1", Labels: []string{model.IssueLabelActive}},
		}
		state.Runs["run-1"] = model.RunRequest{
			ID:        "run-1",
			IssueID:   "issue-1",
			Status:    model.RunStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:      "attempt-1",
			RunID:   "run-1",
			Stage:   "implement",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	meta := map[string]any{
		"current_state": "EXECUTE_STEPS",
		"current_step":  "implement_components",
		"status":        "succeeded",
	}
	if err := service.Heartbeat("attempt-1", 30*time.Second, "running implement step", meta); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	attempt := state.Attempts["attempt-1"]
	if attempt.Metadata["runtime_summary"] != "running implement step" {
		t.Fatalf("unexpected runtime summary: %+v", attempt.Metadata)
	}
	runtimeState, ok := attempt.Metadata["runtime_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime_state map, got %+v", attempt.Metadata)
	}
	if runtimeState["current_state"] != "EXECUTE_STEPS" {
		t.Fatalf("unexpected runtime_state: %+v", runtimeState)
	}
	run := state.Runs["run-1"]
	stageStates, ok := run.Metadata["current_stage_states"].(map[string]any)
	if !ok {
		t.Fatalf("expected current_stage_states in run metadata, got %+v", run.Metadata)
	}
	stageState, ok := stageStates["implement"].(map[string]any)
	if !ok {
		t.Fatalf("expected implement stage state, got %+v", stageStates)
	}
	if stageState["summary"] != "running implement step" {
		t.Fatalf("unexpected stage state summary: %+v", stageState)
	}
}

func TestCompleteRecordsGeneratorAndPromotionCommits(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{ID: "issue-1", Labels: []string{model.IssueLabelActive}},
		}
		state.Runs["run-1"] = model.RunRequest{ID: "run-1", IssueID: "issue-1", Status: model.RunStatusActive, CreatedAt: now, UpdatedAt: now}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:      "attempt-1",
			RunID:   "run-1",
			Stage:   "promote_local",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	outputs := map[string]any{
		"generator_commit":        "sha256:gen",
		"promotion_gitops_commit": "sha256:promo",
	}
	outputBytes, _ := json.Marshal(outputs)
	result := model.StageResult{
		Status:  model.AttemptStatusSucceeded,
		Summary: "promoted",
		Outputs: outputBytes,
		Stats: model.AttemptStats{
			Stage:      "promote_local",
			DurationMS: 400,
			Cost:       model.CostBreakdown{Currency: "USD", TotalUSD: 0.003},
		},
	}
	artifacts := []model.ArtifactRef{{Name: "result"}}
	if err := service.Complete("attempt-1", result, artifacts); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	attempt := state.Attempts["attempt-1"]
	if attempt.Metadata["generator_commit"] != "sha256:gen" {
		t.Fatalf("expected generator_commit metadata, got %v", attempt.Metadata)
	}
	if attempt.Metadata["promotion_gitops_commit"] != "sha256:promo" {
		t.Fatalf("expected promotion_gitops_commit metadata, got %v", attempt.Metadata)
	}
	run := state.Runs["run-1"]
	genCommits, _ := run.Metadata["generator_commits"].([]any)
	if len(genCommits) != 1 || genCommits[0] != "sha256:gen" {
		t.Fatalf("expected generator_commits list, got %v", run.Metadata["generator_commits"])
	}
	promoCommits, _ := run.Metadata["promotion_commits"].([]any)
	if len(promoCommits) != 1 || promoCommits[0] != "sha256:promo" {
		t.Fatalf("expected promotion_commits list, got %v", run.Metadata["promotion_commits"])
	}
}

func TestEnsureRunPersistsWorkOrderCommit(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:               "issue-wo",
		Labels:           []string{model.IssueLabelRequested},
		PipelineTemplate: "default-v1",
		WorkOrder: model.WorkOrder{
			ID:               "wo-issue-wo",
			SourceIssueID:    "issue-wo",
			PipelineTemplate: "default-v1",
			Delivery: model.DeliveryTarget{
				ApplicationRepo: model.RepoTarget{
					ProjectPath:         "group/repo",
					DefaultBranch:       "main",
					WorkingBranchPrefix: "autodev",
				},
				Environments: model.PromotionTargets{
					Local: model.EnvironmentTarget{
						Name: "local",
						GitOpsRepo: model.GitOpsTarget{
							ProjectPath:     "group/repo-gitops",
							Environment:     "local",
							Path:            "clusters/local/repo",
							PromotionBranch: "main",
							Cluster:         "kind-local",
						},
					},
				},
			},
		},
	})
	if err := service.EnqueueFromGitLab(); err != nil {
		t.Fatal(err)
	}
	run, err := service.EnsureRunForIssue("issue-wo")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" {
		t.Fatalf("expected run to exist")
	}
	commit, ok := run.Metadata["work_order_commit"].(string)
	if !ok || commit == "" {
		t.Fatalf("missing work_order_commit metadata: %v", run.Metadata)
	}
	file := filepath.Join(workOrderDir, "work-orders", canonicalWorkOrderID(run), "work-order.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read work-order file: %v", err)
	}
	var persisted model.WorkOrder
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode work order: %v", err)
	}
	if persisted.SourceIssueID != "issue-wo" {
		t.Fatalf("unexpected work order source %s", persisted.SourceIssueID)
	}
}

func TestReconcileFailsRunMissingPipelinePlan(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:               "issue-plan",
		Labels:           []string{model.IssueLabelActive},
		PipelineTemplate: "default-v1",
		WorkOrder: model.WorkOrder{
			ID:               "wo-issue-plan",
			SourceIssueID:    "issue-plan",
			PipelineTemplate: "default-v1",
			Delivery: model.DeliveryTarget{
				ApplicationRepo: model.RepoTarget{
					ProjectPath:         "group/repo",
					DefaultBranch:       "main",
					WorkingBranchPrefix: "autodev",
				},
				Environments: model.PromotionTargets{
					Local: model.EnvironmentTarget{
						Name: "local",
						GitOpsRepo: model.GitOpsTarget{
							ProjectPath:     "group/repo-gitops",
							Environment:     "local",
							Path:            "clusters/local/repo",
							PromotionBranch: "main",
							Cluster:         "kind-local",
						},
					},
				},
			},
		},
	})
	now := time.Now().UTC()
	err := service.store.Save(func(state *model.PersistedState) error {
		state.Issues["issue-plan"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{
				ID:               "issue-plan",
				Labels:           []string{model.IssueLabelActive},
				PipelineTemplate: "default-v1",
				WorkOrder: model.WorkOrder{
					ID:               "wo-issue-plan",
					SourceIssueID:    "issue-plan",
					PipelineTemplate: "default-v1",
					Delivery: model.DeliveryTarget{
						ApplicationRepo: model.RepoTarget{
							ProjectPath:         "group/repo",
							DefaultBranch:       "main",
							WorkingBranchPrefix: "autodev",
						},
						Environments: model.PromotionTargets{
							Local: model.EnvironmentTarget{
								Name: "local",
								GitOpsRepo: model.GitOpsTarget{
									ProjectPath:     "group/repo-gitops",
									Environment:     "local",
									Path:            "clusters/local/repo",
									PromotionBranch: "main",
									Cluster:         "kind-local",
								},
							},
						},
					},
				},
			},
		}
		state.Runs["run-plan"] = model.RunRequest{
			ID:               "run-plan",
			IssueID:          "issue-plan",
			WorkOrder:        state.Issues["issue-plan"].DeliveryIssue.CanonicalWorkOrder(),
			PipelineTemplate: "default-v1",
			Target:           state.Issues["issue-plan"].DeliveryIssue.CanonicalWorkOrder().Delivery,
			Status:           model.RunStatusActive,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = service.Reconcile()
	if err == nil {
		t.Fatal("expected reconcile to fail when a run is missing a materialized pipeline plan")
	}
	if got := err.Error(); got != "run run-plan missing materialized pipeline execution plan" {
		t.Fatalf("unexpected reconcile error: %s", got)
	}
}

func TestCompleteWritesStageReportsIntoWorkOrderRepo(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-1",
		Labels: []string{model.IssueLabelActive},
		WorkOrder: model.WorkOrder{
			ID:            "wo-issue-1",
			SourceIssueID: "issue-1",
		},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-1"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{
				ID:     "issue-1",
				Labels: []string{model.IssueLabelActive},
				WorkOrder: model.WorkOrder{
					ID:            "wo-issue-1",
					SourceIssueID: "issue-1",
				},
			},
		}
		state.Runs["run-1"] = model.RunRequest{
			ID:        "run-1",
			IssueID:   "issue-1",
			Status:    model.RunStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
			WorkOrder: model.WorkOrder{
				ID:            "wo-issue-1",
				SourceIssueID: "issue-1",
			},
		}
		state.Attempts["attempt-1"] = model.StageAttempt{
			ID:      "attempt-1",
			RunID:   "run-1",
			Stage:   "test",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	result := model.StageResult{
		Status:  model.AttemptStatusSucceeded,
		Summary: "tests passed",
		Outputs: mustJSON(t, map[string]any{"suite": "unit", "passed": true}),
		Stats: model.AttemptStats{
			Stage:      "test",
			DurationMS: 123,
		},
	}
	artifacts := []model.ArtifactRef{{Name: "result", URI: "/tmp/result.json"}}
	if err := service.Complete("attempt-1", result, artifacts); err != nil {
		t.Fatal(err)
	}

	reportPath := filepath.Join(workOrderDir, "work-orders", "wo-issue-1", "runs", "run-1", "stages", "test", "attempt-01", "report.json")
	summaryPath := filepath.Join(workOrderDir, "work-orders", "wo-issue-1", "runs", "run-1", "stages", "test", "attempt-01", "summary.json")
	indexPath := filepath.Join(workOrderDir, "work-orders", "wo-issue-1", "runs", "run-1", "index.json")
	for _, path := range []string{reportPath, summaryPath, indexPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestCompletePersistsPipelinePlanEvidenceOnRun(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, root := newTestServiceWithWorkOrderDir(t, workOrderDir)
	writeIssue(t, root, model.DeliveryIssue{
		ID:     "issue-plan",
		Labels: []string{model.IssueLabelActive},
		WorkOrder: model.WorkOrder{
			ID:            "wo-plan",
			SourceIssueID: "issue-plan",
		},
	})
	err := service.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		state.Issues["issue-plan"] = model.TrackedIssue{
			DeliveryIssue: model.DeliveryIssue{
				ID:     "issue-plan",
				Labels: []string{model.IssueLabelActive},
				WorkOrder: model.WorkOrder{
					ID:            "wo-plan",
					SourceIssueID: "issue-plan",
				},
			},
		}
		state.Runs["run-plan"] = model.RunRequest{
			ID:        "run-plan",
			IssueID:   "issue-plan",
			Status:    model.RunStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
			WorkOrder: model.WorkOrder{
				ID:            "wo-plan",
				SourceIssueID: "issue-plan",
			},
		}
		state.Attempts["attempt-plan"] = model.StageAttempt{
			ID:      "attempt-plan",
			RunID:   "run-plan",
			Stage:   "plan",
			Attempt: 1,
			Status:  model.AttemptStatusRunning,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	result := model.StageResult{
		Status:  model.AttemptStatusSucceeded,
		Summary: "plan ready",
		Outputs: mustJSON(t, map[string]any{
			"pipeline_intent":         validPipelineIntentForTest(),
			"policy_evaluation":       validPolicyEvaluationForTest(),
			"pipeline_build_plan":     validPipelineBuildPlanForTest(),
			"pipeline_execution_plan": validPipelineExecutionPlanForTest([]map[string]any{{"name": "plan"}, {"name": "implement", "dependencies": []string{"plan"}}}),
		}),
	}
	if err := service.Complete("attempt-plan", result, nil); err != nil {
		t.Fatal(err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	run := state.Runs["run-plan"]
	for _, key := range []string{"pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan"} {
		if _, ok := run.Metadata[key]; !ok {
			t.Fatalf("expected %s in run metadata, got %v", key, run.Metadata)
		}
	}
	plan, ok := service.pipelinePlanForRun(run)
	if !ok {
		t.Fatal("expected persisted pipeline execution plan")
	}
	if len(plan.Stages) != 2 || plan.Stages[1].Name != "implement" {
		t.Fatalf("unexpected execution plan: %+v", plan)
	}
	for _, rel := range []string{
		"work-orders/wo-plan/runs/run-plan/pipeline/pipeline_intent.json",
		"work-orders/wo-plan/runs/run-plan/pipeline/policy_evaluation.json",
		"work-orders/wo-plan/runs/run-plan/pipeline/pipeline_build_plan.json",
		"work-orders/wo-plan/runs/run-plan/pipeline/pipeline_execution_plan.json",
	} {
		if _, err := os.Stat(filepath.Join(workOrderDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
}

func TestRunnableStagesPrefersPipelinePlan(t *testing.T) {
	service, _ := newTestService(t)
	plan := model.PipelineExecutionPlan{
		RunID: "run-plan",
		Stages: []model.PipelineExecutionStage{
			{
				Name:            "meta-stage",
				Image:           "registry.mph.tech/meta:latest",
				Entrypoint:      []string{"/bin/sh", "-c", "true"},
				Container:       model.StageContainer{RunAs: model.ExecutionIdentityAgent},
				QueueMode:       model.StageQueueModeAuto,
				SuccessCriteria: model.StageSuccessCriteria{RequireSummary: true, RequiredOutputs: []string{"summary"}},
			},
		},
	}
	run := model.RunRequest{ID: "run-plan", IssueID: "issue-plan", Status: model.RunStatusActive, WorkOrder: model.WorkOrder{ID: "wo-plan"}}
	writePlanToJournal(t, service.workOrderRepo, run, plan)
	stages := pipeline.PlanRunnableStages(&plan, map[string]model.StageAttempt{}, true)
	if len(stages) != 1 || stages[0].Name != "meta-stage" {
		t.Fatalf("expected runnable stage meta-stage, got %#v", stages)
	}
}

func TestRunStatusUsesPipelinePlanCheckpoint(t *testing.T) {
	service, _ := newTestService(t)
	plan := model.PipelineExecutionPlan{
		RunID: "run-plan",
		Stages: []model.PipelineExecutionStage{
			{
				Name:            "meta-stage",
				Checkpoint:      model.RunStatusAwaitingApproval,
				Container:       model.StageContainer{RunAs: model.ExecutionIdentityGoverned},
				Image:           "registry.mph.tech/meta:latest",
				Entrypoint:      []string{"/bin/sh", "-c", "true"},
				SuccessCriteria: model.StageSuccessCriteria{RequireSummary: true},
			},
		},
	}
	run := model.RunRequest{ID: "run-plan", IssueID: "issue-plan", Status: model.RunStatusActive, WorkOrder: model.WorkOrder{ID: "wo-plan"}}
	writePlanToJournal(t, service.workOrderRepo, run, plan)
	byStage := map[string]model.StageAttempt{
		"meta-stage": {Stage: "meta-stage", Status: model.AttemptStatusSucceeded},
	}
	status := pipeline.RunStatusFromPlan(&plan, byStage, run.DeliveryTarget(), false)
	if status != model.RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval, got %s", status)
	}
}

func TestPipelinePlanRehydratedFromJournal(t *testing.T) {
	workOrderDir := filepath.Join(t.TempDir(), "workorders")
	service, _ := newTestServiceWithWorkOrderDir(t, workOrderDir)
	run := model.RunRequest{
		ID:      "run-plan",
		IssueID: "issue-plan",
		WorkOrder: model.WorkOrder{
			ID:            "wo-plan",
			SourceIssueID: "issue-plan",
		},
	}
	plan := model.PipelineExecutionPlan{
		SchemaVersion:          "autodev-pipeline-execution-plan-v1",
		RunID:                  run.ID,
		IssueID:                run.IssueID,
		WorkOrderID:            run.WorkOrder.ID,
		IssueType:              "bug_fix",
		PipelineFamily:         "default-v1",
		PipelineSelection:      "selected",
		Testing: model.TestingPolicy{
			Strategy:          "tests-before-implementation",
			Immutable:         true,
			ReadableByAgent:   false,
			ExecutableByAgent: true,
		},
		DeliveryName: "demo",
		Stages: []model.PipelineExecutionStage{
			{
				Name:            "plan",
				Dependencies:    []string{},
				QueueMode:       model.StageQueueModeAuto,
				Transitions:     model.StageTransition{},
				Image:           "autodev-stage-plan:d2ab0adcd5d1ae7cc8e3b40b3813cafc9438e5a0",
				ImageDigest:     "sha256:plan",
				Entrypoint:      []string{"autodev-stage-runtime"},
				Container:       model.StageContainer{},
				SuccessCriteria: model.StageSuccessCriteria{},
				OutputArtifacts: []model.StageOutputArtifact{},
				ReportStages:    []string{},
			},
		},
	}
	paths := workorder.PipelineArtifactPaths(run)
	planPath := filepath.Join(workOrderDir, paths["pipeline_execution_plan"])
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := contracts.WriteFile(planPath, contracts.PipelineExecutionPlanSchema, plan); err != nil {
		t.Fatal(err)
	}
	if gotPlan, ok := service.pipelinePlanForRun(run); !ok {
		t.Fatal("expected plan rehydrated from journal")
	} else if gotPlan.RunID != plan.RunID {
		t.Fatalf("expected run id %s, got %s", plan.RunID, gotPlan.RunID)
	}
}

func TestClaimNextForIssueFiltersPendingAttempts(t *testing.T) {
	service, root := newTestService(t)
	writeIssue(t, root, model.DeliveryIssue{ID: "issue-a", Labels: []string{model.IssueLabelActive}})
	writeIssue(t, root, model.DeliveryIssue{ID: "issue-b", Labels: []string{model.IssueLabelActive}})
	now := time.Now().UTC()
	runA := model.RunRequest{ID: "run-a", IssueID: "issue-a", Status: model.RunStatusActive, CreatedAt: now, UpdatedAt: now}
	runB := model.RunRequest{ID: "run-b", IssueID: "issue-b", Status: model.RunStatusActive, CreatedAt: now, UpdatedAt: now}
	intakeSpec, ok := service.specs["intake"]
	if !ok {
		t.Fatal("missing intake spec")
	}
	writePlanToJournal(t, service.workOrderRepo, runA, mustPlanForRun(t, runA, intakeSpec))
	writePlanToJournal(t, service.workOrderRepo, runB, mustPlanForRun(t, runB, intakeSpec))
	err := service.store.Save(func(state *model.PersistedState) error {
		state.Issues["issue-a"] = model.TrackedIssue{DeliveryIssue: model.DeliveryIssue{ID: "issue-a", Labels: []string{model.IssueLabelActive}}}
		state.Issues["issue-b"] = model.TrackedIssue{DeliveryIssue: model.DeliveryIssue{ID: "issue-b", Labels: []string{model.IssueLabelActive}}}
		state.Runs["run-a"] = runA
		state.Runs["run-b"] = runB
		state.Attempts["attempt-a"] = model.StageAttempt{ID: "attempt-a", RunID: "run-a", Stage: "intake", Attempt: 1, Status: model.AttemptStatusPending}
		state.Attempts["attempt-b"] = model.StageAttempt{ID: "attempt-b", RunID: "run-b", Stage: "intake", Attempt: 1, Status: model.AttemptStatusPending}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	attempt, spec, err := service.ClaimNextForIssue("worker-1", 30*time.Second, nil, "issue-b")
	if err != nil {
		t.Fatal(err)
	}
	if attempt == nil {
		t.Fatal("expected attempt to be claimed")
	}
	if attempt.ID != "attempt-b" {
		t.Fatalf("expected attempt-b, got %s", attempt.ID)
	}
	if spec.Name != "intake" {
		t.Fatalf("expected intake spec, got %s", spec.Name)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Attempts["attempt-a"].Status != model.AttemptStatusPending {
		t.Fatalf("expected attempt-a to stay pending, got %s", state.Attempts["attempt-a"].Status)
	}
	if state.Attempts["attempt-b"].Status != model.AttemptStatusRunning {
		t.Fatalf("expected attempt-b to be running, got %s", state.Attempts["attempt-b"].Status)
	}
}

func newTestService(t *testing.T) (*Service, string) {
	return newTestServiceWithWorkOrderDir(t, filepath.Join(t.TempDir(), "workorders"))
}

func newTestServiceWithWorkOrderDir(t *testing.T, workOrderDir string) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installFixedConfigForServiceTests(t, filepath.Join(repoRoot, "TEST"))
	specs, err := configsource.LoadStageSpecs()
	if err != nil {
		t.Fatal(err)
	}
	return New(
		store.New(filepath.Join(root, "state")),
		gitlab.NewFilesystemAdapter(filepath.Join(root, "gitlab")),
		locks.NoopManager{},
		signals.NewService(signals.NoopStore{}),
		specs,
		workOrderDir,
	), root
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeIssue(t *testing.T, root string, issue model.DeliveryIssue) {
	t.Helper()
	dir := filepath.Join(root, "gitlab", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, issue.ID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func installFixedConfigForServiceTests(t *testing.T, sourceRoot string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	targetRoot := filepath.Join(home, ".autodev", "config")
	if err := copyTreeForServiceTests(sourceRoot, targetRoot); err != nil {
		t.Fatalf("install fixed config: %v", err)
	}
}

func copyTreeForServiceTests(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func writePlanToJournal(t *testing.T, workOrderDir string, run model.RunRequest, plan model.PipelineExecutionPlan) {
	t.Helper()
	paths := workorder.PipelineArtifactPaths(run)
	planPath := filepath.Join(workOrderDir, paths["pipeline_execution_plan"])
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustPlanForRun(t *testing.T, run model.RunRequest, specs ...model.StageSpec) model.PipelineExecutionPlan {
	t.Helper()
	if run.IssueID == "" {
		run.IssueID = "issue-test"
	}
	if run.WorkOrder.ID == "" {
		run.WorkOrder.ID = "wo-test"
	}
	if run.WorkOrder.IssueType == "" {
		run.WorkOrder.IssueType = "bug_fix"
	}
	if run.WorkOrder.PipelineTemplate == "" {
		run.WorkOrder.PipelineTemplate = "default-v1"
	}
	if run.WorkOrder.Delivery.Name == "" {
		run.WorkOrder.Delivery.Name = "demo"
	}
	if run.WorkOrder.Testing.Strategy == "" {
		run.WorkOrder.Testing = model.TestingPolicy{
			Strategy:          "tests-before-implementation",
			Immutable:         true,
			ReadableByAgent:   false,
			ExecutableByAgent: true,
		}
	}
	images, err := stagecontainer.ResolveRuntimeImages(stageNames(specs))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pipeline.BuildExecutionPlan(run, specs, images)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func stageNames(specs []model.StageSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

func validPipelineIntentForTest() map[string]any {
	return map[string]any{
		"schema_version":      "autodev-pipeline-intent-v1",
		"run_id":              "run-test",
		"issue_id":            "issue-test",
		"work_order_id":       "wo-test",
		"issue_type":          "bug_fix",
		"pipeline_family":     "bugfix",
		"pipeline_selection":  "selected",
		"policy_profile":      "default",
		"delivery_name":       "demo",
		"requested_outcome":   "fix bug",
		"deploy_as_unit":      false,
		"selected_components": []string{"api"},
		"ordered_components":  []string{"api"},
		"documentation":       map[string]any{"required": false, "docs_component": ""},
		"testing":             map[string]any{"strategy": "tests-before-implementation", "immutable": true, "readable_by_agent": false, "executable_by_agent": true},
		"environments":        []map[string]any{{"name": "local", "project_path": "group/gitops", "path": "clusters/local/demo", "cluster": "local", "approval_required": false}},
	}
}

func validPolicyEvaluationForTest() map[string]any {
	return map[string]any{
		"schema_version": "autodev-policy-evaluation-v1",
		"run_id":         "run-test",
		"issue_id":       "issue-test",
		"work_order_id":  "wo-test",
		"policy_profile": "default",
		"outcome":        "approved",
		"pipeline_scope": []map[string]any{},
		"hierarchy":      []map[string]any{},
		"stage_scope":    map[string]any{},
		"stage_policies": []map[string]any{},
	}
}

func validPipelineBuildPlanForTest() map[string]any {
	return map[string]any{
		"schema_version":           "autodev-pipeline-build-plan-v1",
		"run_id":                   "run-test",
		"issue_id":                 "issue-test",
		"work_order_id":            "wo-test",
		"issue_type":               "bug_fix",
		"pipeline_family":          "bugfix",
		"delivery_name":            "demo",
		"images": []map[string]any{{
			"stage":        "plan",
			"image_ref":    "autodev-stage-plan:current",
			"image_digest": "sha256:plan",
			"container":    map[string]any{},
			"entrypoint":   []string{"autodev-stage-runtime"},
			"tooling_repo": map[string]any{"url": "tooling/plan", "ref": "main"},
			"prompt_file":  "prompts/plan.md",
		}},
	}
}

func validPipelineExecutionPlanForTest(stages []map[string]any) map[string]any {
	if len(stages) == 0 {
		stages = []map[string]any{{"name": "plan"}}
	}
	normalized := make([]map[string]any, 0, len(stages))
	for _, stage := range stages {
		entry := map[string]any{
			"name":             stage["name"],
			"dependencies":     []string{},
			"queue_mode":       "auto",
			"transitions":      map[string]any{},
			"container":        map[string]any{},
			"success_criteria": map[string]any{},
			"output_artifacts": []map[string]any{},
			"report_stages":    []string{},
			"image_ref":        "autodev-stage-" + stage["name"].(string) + ":current",
			"image_digest":     "sha256:" + stage["name"].(string),
			"entrypoint":       []string{"autodev-stage-runtime"},
		}
		if deps, ok := stage["dependencies"]; ok {
			entry["dependencies"] = deps
		}
		normalized = append(normalized, entry)
	}
	return map[string]any{
		"schema_version":           "autodev-pipeline-execution-plan-v1",
		"run_id":                   "run-test",
		"issue_id":                 "issue-test",
		"work_order_id":            "wo-test",
		"issue_type":               "bug_fix",
		"pipeline_family":          "bugfix",
		"pipeline_selection":       "selected",
		"testing":                  map[string]any{"strategy": "tests-before-implementation", "immutable": true, "readable_by_agent": false, "executable_by_agent": true},
		"delivery_name":            "demo",
		"stages":                   normalized,
	}
}
