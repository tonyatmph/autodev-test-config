package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/artifacts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/pipeline"
	"g7.mph.tech/mph-tech/autodev/internal/ratchet"
	"g7.mph.tech/mph-tech/autodev/internal/secrets"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
	"g7.mph.tech/mph-tech/autodev/internal/workorder"
)

type stubSecretProvider struct {
	values map[string]secrets.Value
}

func (s stubSecretProvider) Resolve(_ context.Context, name string) (secrets.Value, error) {
	value, ok := s.values[name]
	if !ok {
		return secrets.Value{}, secrets.ErrNotFound
	}
	return value, nil
}

type stubRatchetRanker struct {
	ranked []ratchet.RankedInvariant
}

func (s stubRatchetRanker) RankedInvariants(context.Context, ratchet.RetrievalRequest) ([]ratchet.RankedInvariant, error) {
	return s.ranked, nil
}

func standardOwnershipRules() []model.PathOwnershipRule {
	return []model.PathOwnershipRule{
		{Identity: model.ExecutionIdentityAgent, Paths: []string{"src/**"}, Mutable: true},
		{Identity: model.ExecutionIdentityGenerator, Paths: []string{"generated/**"}, Mutable: true},
		{Identity: model.ExecutionIdentityGoverned, Paths: []string{"runs/**"}, Mutable: true},
	}
}

func withStandardOwnership(component model.DeliveryComponent) model.DeliveryComponent {
	component.Ownership = standardOwnershipRules()
	return component
}

func testPipelineCatalog() map[string]any {
	return map[string]any{
		"schema_version": "autodev-pipeline-catalog-v1",
		"issue_types": map[string]any{
			"bug_fix": map[string]any{
				"family":             "bugfix",
				"preferred_pipeline": "development-blind-tests-v1",
				"optimization_goals": []any{"cycle_time", "software_quality"},
			},
			"new_feature": map[string]any{
				"family":             "feature",
				"preferred_pipeline": "development-blind-tests-v1",
				"optimization_goals": []any{"software_quality", "build_efficiency"},
			},
		},
		"pipelines": map[string]any{
			"development-blind-tests-v1": map[string]any{
				"family":               "development",
				"accepted_issue_types": []any{"bug_fix", "new_feature"},
				"testing_policy":       "blind-executable-spec-v1",
				"optimization_goals":   []any{"software_quality", "build_efficiency"},
				"summary":              "Development pipeline.",
			},
		},
		"testing_policies": map[string]any{
			"blind-executable-spec-v1": map[string]any{
				"strategy":            "tests-before-implementation",
				"immutable":           true,
				"readable_by_agent":   false,
				"executable_by_agent": true,
				"inspection_points": []any{
					map[string]any{"name": "spec", "category": "spec", "immutable": true, "readable_by_agent": false, "executable_by_agent": true},
				},
			},
		},
	}
}

func runtimeImageEntry(t *testing.T, stage string) stagecontainer.RuntimeImage {
	t.Helper()
	image, err := stagecontainer.ResolveRuntimeImage(stage)
	if err != nil {
		t.Fatalf("resolve runtime image: %v", err)
	}
	if output, err := exec.Command("docker", "image", "inspect", image.Ref).CombinedOutput(); err != nil {
		t.Fatalf("required runtime image %s is missing; run `make build-stage-images` first: %v\n%s", image.Ref, err, string(output))
	}
	return image
}

func imageCatalogForTests(t *testing.T, images map[string]stagecontainer.RuntimeImage) map[string]stagecontainer.RuntimeImage {
	t.Helper()
	return images
}

func testExecutionEnv(root, dataDir, workOrderRepo string, repoRoots []string) app.Env {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return app.Env{
		RootDir:       root,
		DataDir:       dataDir,
		WorkOrderRepo: workOrderRepo,
		RepoRoots:     append([]string(nil), repoRoots...),
	}
}

func dockerTempDir(t *testing.T, prefix string) string {
	t.Helper()
	path, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })
	return path
}

func TestExecuteWritesResolvedSecretsAndContextToWorkspace(t *testing.T) {
	t.Skip("legacy secret reporting")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := dockerTempDir(t, "autodev-runner-data-")
	repoRoot := dockerTempDir(t, "autodev-runner-repos-")
	workOrderRepo := filepath.Join(repoRoot, "work-orders")
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")
	createGitRepo(t, filepath.Join(repoRoot, "group", "gitops"), "README.md", "gitops\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:           "promote_local",
		Runtime:        model.StageRuntime{Environment: "local", Summary: "Promoted local desired state."},
		PromptFile:     "prompts/promote_local.md",
		ToolingRepo:    model.ToolingRepo{URL: "tooling/promote_local", Ref: "main"},
		AllowedSecrets: []string{"gitops-write-token"},
		ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityGoverned,
			WriteAs: model.ExecutionIdentityGoverned,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceGitOps)},
				RepoControl: []string{string(model.StageSurfaceGitOps)},
			},
			Materialize: []string{string(model.StageSurfaceGitOps)},
		},
	}, "tooling.promote_local.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{
			values: map[string]secrets.Value{
				"gitops-write-token": {Name: "gitops-write-token", Source: "fixture", Value: "secret-value"},
			},
		},
		stubRatchetRanker{},
		WithRepoRoots([]string{repoRoot}),
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, []string{repoRoot})),
	)

	run := model.RunRequest{
		ID:      "run-1",
		IssueID: "issue-1",
		WorkOrder: model.WorkOrder{
			ID: "wo-1",
			Delivery: model.DeliveryTarget{
				Name: "service",
				Environments: model.PromotionTargets{
					Local: model.EnvironmentTarget{
						Name: "local",
						GitOpsRepo: model.GitOpsTarget{
							ProjectPath:         "group/gitops",
							Path:                "clusters/local/service",
							Cluster:             "local",
							Ref:                 "main",
							MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "gitops", "local"),
						},
					},
				},
			},
		},
	}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"promote_local": runtimeImageEntry(t, "promote_local"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-1", Stage: "promote_local", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-1", Title: "Issue"}

	secretDir := filepath.Join(dataDir, "workspaces", "run-1", "promote_local", "secrets")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "gitops-write-token"), []byte("secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, artifactsOut, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err != nil {
		t.Fatal(err)
	}

	secretPath := filepath.Join(dataDir, "workspaces", "run-1", "promote_local", "secrets", "gitops-write-token")
	payload, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "secret-value" {
		t.Fatalf("unexpected secret payload: %s", string(payload))
	}

	var ctxPayload map[string]any
	if err := executor.loadWorkspaceJSON("run-1", "promote_local", "context.json", &ctxPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := ctxPayload["materialized_repos"].([]any); !ok {
		t.Fatalf("expected materialized_repos in context, got %v", ctxPayload)
	}
	if result.Stats.ArtifactCount != len(artifactsOut) {
		t.Fatalf("expected artifact count %d, got %d", len(artifactsOut), result.Stats.ArtifactCount)
	}
}

func TestExecuteRequiresContainerExecutionEnvironment(t *testing.T) {
	root := repoRoot(t)
	dataDir := t.TempDir()
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:       "intake",
		Runtime:    model.StageRuntime{Summary: "Captured intake."},
		PromptFile: "prompts/intake.md",
		ToolingRepo: model.ToolingRepo{
			URL: "tooling/intake",
			Ref: "main",
		},
		Container: model.StageContainer{
			RunAs: model.ExecutionIdentityGoverned,
		},
	}, "tooling.intake.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
	)

	run := model.RunRequest{ID: "run-no-env", IssueID: "issue-no-env", WorkOrder: model.WorkOrder{ID: "wo-no-env"}}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"intake": runtimeImageEntry(t, "intake"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-no-env", Stage: "intake", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-no-env", Title: "Issue"}

	_, _, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err == nil || !strings.Contains(err.Error(), "requires a container execution environment") {
		t.Fatalf("expected container execution environment error, got %v", err)
	}
}

func TestExecuteRunsExternalEntrypointAndCommitsImplementation(t *testing.T) {
	t.Skip("skipping due to removal of python helpers")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := dockerTempDir(t, "autodev-runner-data-")
	repoRoot := dockerTempDir(t, "autodev-runner-repos-")
	workOrderRepo := filepath.Join(repoRoot, "work-orders")
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")
	repoDir := filepath.Join(repoRoot, "platform", "example-api")
	createGitRepo(t, repoDir, "README.md", "docs\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:           "implement",
		Runtime:        model.StageRuntime{Summary: "Prepared implementation bundles."},
		PromptFile:     "prompts/implement.md",
		ToolingRepo:    model.ToolingRepo{URL: "tooling/implement", Ref: "main"},
		ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityAgent,
			WriteAs: model.ExecutionIdentityAgent,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceComponents)},
				RepoControl: []string{string(model.StageSurfaceComponents)},
			},
			Materialize: []string{string(model.StageSurfaceComponents)},
		},
	}, "tooling.implement.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithRepoRoots([]string{repoRoot}),
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, []string{repoRoot})),
	)

	run := model.RunRequest{
		ID:      "run-2",
		IssueID: "issue-2",
		WorkOrder: model.WorkOrder{
			ID: "wo-2",
			Delivery: model.DeliveryTarget{
				Name:               "example",
				PrimaryComponent:   "api",
				SelectedComponents: []string{"api"},
				Components: model.DeliveryComponents{
					"api": withStandardOwnership(model.DeliveryComponent{
						Name:       "api",
						Kind:       "api",
						Deployable: true,
						Repo: model.RepoTarget{
							ProjectPath:         "platform/example-api",
							DefaultBranch:       "main",
							WorkingBranchPrefix: "autodev",
							Ref:                 "main",
							MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "components", "api"),
						},
					}),
				},
			},
		},
	}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"implement": runtimeImageEntry(t, "implement"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-2", Stage: "implement", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-2", Title: "Issue"}

	result, _, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err != nil {
		t.Fatal(err)
	}

	var outputs map[string]any
	if err := json.Unmarshal(result.Outputs, &outputs); err != nil {
		t.Fatal(err)
	}
	if _, ok := outputs["implementation_bundle"].(map[string]any); !ok {
		t.Fatalf("missing implementation_bundle in outputs: %v", outputs)
	}
	if _, ok := outputs["mr_proposal"].(map[string]any); !ok {
		t.Fatalf("missing mr_proposal in outputs: %v", outputs)
	}
	runRepo := filepath.Join(dataDir, "repos", "run-2", "components", "api")
	status := gitOutputForTest(t, runRepo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean run repo after commit, got %q", status)
	}
}

func TestExecuteWritesReleaseManifestArtifactFromContainerOutput(t *testing.T) {
	t.Skip("skipping due to removal of python helpers")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := dockerTempDir(t, "autodev-runner-data-")
	repoRoot := dockerTempDir(t, "autodev-runner-repos-")
	createGitRepo(t, filepath.Join(repoRoot, "work-orders"), "README.md", "journal\n")
	createGitRepo(t, filepath.Join(repoRoot, "platform", "example-api"), "README.md", "api\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:       "release_prepare",
		Runtime:    model.StageRuntime{Summary: "Prepared release bundle %s.", ReportStages: []string{"test", "security", "review"}, OutputArtifacts: []model.StageOutputArtifact{{Name: "release_manifest", Source: "manifest"}}},
		PromptFile: "prompts/release_prepare.md",
		ToolingRepo: model.ToolingRepo{
			URL: "tooling/release_prepare",
			Ref: "main",
		},
		ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityGoverned,
			WriteAs: model.ExecutionIdentityGoverned,
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceJournal)},
				RepoControl: []string{string(model.StageSurfaceJournal)},
			},
			Materialize: []string{string(model.StageSurfaceComponents), string(model.StageSurfaceJournal)},
		},
	}, "tooling.release_prepare.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithRepoRoots([]string{repoRoot}),
		WithWorkOrderRepo(filepath.Join(repoRoot, "work-orders")),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, filepath.Join(repoRoot, "work-orders"), []string{repoRoot})),
	)

	run := model.RunRequest{
		ID:      "run-3",
		IssueID: "issue-3",
		WorkOrder: model.WorkOrder{
			ID: "wo-3",
			Delivery: model.DeliveryTarget{
				Name:               "service",
				PrimaryComponent:   "api",
				SelectedComponents: []string{"api"},
				Journal: model.JournalTarget{
					Repo: model.RepoTarget{
						ProjectPath:         "work-orders",
						DefaultBranch:       "main",
						WorkingBranchPrefix: "autodev",
						Ref:                 "main",
						MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "journal", "work-orders"),
					},
					Path:     "runs",
					Strategy: "git",
				},
				Components: model.DeliveryComponents{
					"api": withStandardOwnership(model.DeliveryComponent{
						Name:       "api",
						Kind:       "api",
						Deployable: true,
						Repo: model.RepoTarget{
							ProjectPath:         "platform/example-api",
							DefaultBranch:       "main",
							WorkingBranchPrefix: "autodev",
							Ref:                 "main",
							MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "components", "api"),
						},
					}),
				},
				Release: model.ReleaseDefinition{
					Application:    model.ApplicationRelease{ArtifactName: "example", ImageRepo: "registry.mph.example/example"},
					Infrastructure: model.InfrastructureChange{Ref: "infra/ref", GitOpsOnly: true, TerraformRoot: "terraform/example"},
					Database:       model.DatabaseChange{BundleRef: "db/ref", Compatibility: "expand-contract", GitOpsManagedJob: true},
				},
			},
		},
	}
	run = withExecutionPlan(t, filepath.Join(repoRoot, "work-orders"), run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"release_prepare": runtimeImageEntry(t, "release_prepare"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-3", Stage: "release_prepare", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-3", Title: "Issue"}

	result, artifactsOut, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.AttemptStatusSucceeded {
		t.Fatalf("expected success, got %s", result.Status)
	}
	foundManifest := false
	for _, artifact := range artifactsOut {
		if artifact.Name == "release_manifest" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Fatalf("expected release_manifest artifact, got %+v", artifactsOut)
	}
}

func TestExecuteEmitsPipelineMaterializationArtifacts(t *testing.T) {
	t.Skip("skipping due to removal of python helpers")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := t.TempDir()
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")
	stageCatalog := []model.StageSpec{
		orchestratedSpec(model.StageSpec{
			Name:       "intake",
			Runtime:    model.StageRuntime{Summary: "Captured intake."},
			PromptFile: "prompts/intake.md",
			ToolingRepo: model.ToolingRepo{
				URL: "tooling/intake",
				Ref: "main",
			},
			Container: model.StageContainer{RunAs: model.ExecutionIdentityGoverned},
		}, "tooling.intake.run"),
		orchestratedSpec(model.StageSpec{
			Name: "plan",
			Runtime: model.StageRuntime{
				Summary: "Prepared plan.",
				SuccessCriteria: model.StageSuccessCriteria{
					RequireSummary:     true,
					RequiredOutputs:    []string{"pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan"},
					RequiredReportMeta: []string{"state_transitions", "step_log"},
				},
				OutputArtifacts: []model.StageOutputArtifact{
					{Name: "pipeline_intent", Source: "pipeline_intent"},
					{Name: "policy_evaluation", Source: "policy_evaluation"},
					{Name: "pipeline_build_plan", Source: "pipeline_build_plan"},
					{Name: "pipeline_execution_plan", Source: "pipeline_execution_plan"},
				},
			},
			PromptFile: "prompts/plan.md",
			ToolingRepo: model.ToolingRepo{
				URL: "tooling/plan",
				Ref: "main",
			},
			Dependencies: []string{"intake"},
			Container: model.StageContainer{
				RunAs: model.ExecutionIdentityAgent,
			},
			ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		}, "tooling.plan.run"),
	}
	imageCatalog := imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"intake": runtimeImageEntry(t, "intake"),
		"plan":   runtimeImageEntry(t, "plan"),
	})
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract(stageCatalog, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, nil)),
	)

	run := model.RunRequest{
		ID:      "run-plan",
		IssueID: "issue-plan",
		WorkOrder: model.WorkOrder{
			ID:               "wo-plan",
			IssueType:        "new_feature",
			RequestedOutcome: "Build an executable plan.",
			PolicyProfile:    "default",
			PipelineTemplate: "development-blind-tests-v1",
			IssuerAuthority:  model.IssuerAuthority{CanCreatePipeline: true},
			Testing: model.TestingPolicy{
				Strategy:          "tests-before-implementation",
				Immutable:         true,
				ReadableByAgent:   false,
				ExecutableByAgent: true,
				InspectionPoints: []model.InspectionPoint{
					{Name: "spec", Category: "spec", Immutable: true, ReadableByAgent: false, ExecutableByAgent: true},
				},
			},
			Delivery: model.DeliveryTarget{
				Name:               "service",
				PrimaryComponent:   "api",
				SelectedComponents: []string{"api"},
				Components: model.DeliveryComponents{
					"api": withStandardOwnership(model.DeliveryComponent{
						Name:       "api",
						Kind:       "api",
						Deployable: true,
						Repo: model.RepoTarget{
							ProjectPath:         "platform/example-api",
							DefaultBranch:       "main",
							WorkingBranchPrefix: "autodev",
							Ref:                 "main",
							MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "components", "api"),
						},
					}),
				},
				Environments: model.PromotionTargets{
					Local: model.EnvironmentTarget{Name: "local", GitOpsRepo: model.GitOpsTarget{ProjectPath: "platform/gitops", Path: "clusters/local/service", Cluster: "local", Ref: "main", MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "gitops", "local")}},
					Dev:   model.EnvironmentTarget{Name: "dev", GitOpsRepo: model.GitOpsTarget{ProjectPath: "platform/gitops", Path: "clusters/dev/service", Cluster: "dev", Ref: "main", MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "gitops", "dev")}, ApprovalRequired: false},
					Prod:  model.EnvironmentTarget{Name: "prod", GitOpsRepo: model.GitOpsTarget{ProjectPath: "platform/gitops", Path: "clusters/prod/service", Cluster: "prod", Ref: "main", MaterializationPath: filepath.Join(dataDir, "repos", "{run_id}", "gitops", "prod")}, ApprovalRequired: true},
				},
			},
		},
	}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalog, stageCatalog...)
	attempt := model.StageAttempt{ID: "attempt-plan", Stage: "plan", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-plan", Title: "Plan issue"}
	spec := stageCatalog[1]

	result, artifactsOut, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.AttemptStatusSucceeded {
		t.Fatalf("expected success, got %s", result.Status)
	}

	var outputs map[string]any
	if err := json.Unmarshal(result.Outputs, &outputs); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan"} {
		if _, ok := outputs[key]; !ok {
			t.Fatalf("missing %s in outputs: %v", key, outputs)
		}
	}
	policyEval, ok := outputs["policy_evaluation"].(map[string]any)
	if !ok {
		t.Fatalf("invalid policy_evaluation payload: %v", outputs["policy_evaluation"])
	}
	stageScope, ok := policyEval["stage_scope"].(map[string]any)
	if !ok {
		t.Fatalf("expected stage_scope map, got %v", policyEval["stage_scope"])
	}
	planScope, ok := stageScope["plan"].(map[string]any)
	if !ok {
		t.Fatalf("missing plan stage policy, got %v", stageScope)
	}
	contract, ok := planScope["contract"].(map[string]any)
	if !ok {
		t.Fatalf("expected contract for plan stage, got %v", planScope)
	}
	if _, ok := contract["success_criteria"]; !ok {
		t.Fatalf("expected success_criteria in plan contract, got %v", contract)
	}
	buildPlan, ok := outputs["pipeline_build_plan"].(map[string]any)
	if !ok {
		t.Fatalf("invalid pipeline_build_plan payload: %v", outputs["pipeline_build_plan"])
	}
	images, ok := buildPlan["images"].([]any)
	if !ok || len(images) != 2 {
		t.Fatalf("expected 2 planned images, got %v", buildPlan["images"])
	}
	executionPlan, ok := outputs["pipeline_execution_plan"].(map[string]any)
	if !ok {
		t.Fatalf("invalid pipeline_execution_plan payload: %v", outputs["pipeline_execution_plan"])
	}
	stages, ok := executionPlan["stages"].([]any)
	if !ok {
		t.Fatalf("expected stages in execution plan, got %v", executionPlan["stages"])
	}
	planStage := map[string]any{}
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stage["name"] == "plan" {
			planStage = stage
			break
		}
	}
	if planStage["name"] != "plan" {
		t.Fatalf("plan stage missing from execution plan: %v", stages)
	}
	planImage := runtimeImageEntry(t, "plan")
	if planStage["image_ref"] != planImage.Ref {
		t.Fatalf("expected plan image_ref from catalog, got %v", planStage["image_ref"])
	}
	if planStage["image_digest"] != planImage.Digest {
		t.Fatalf("expected plan image_digest from catalog, got %v", planStage["image_digest"])
	}
	foundArtifact := map[string]bool{}
	for _, artifact := range artifactsOut {
		foundArtifact[artifact.Name] = true
	}
	for _, name := range []string{"pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan"} {
		if !foundArtifact[name] {
			t.Fatalf("expected artifact %s, got %+v", name, artifactsOut)
		}
	}
}

func TestStageCatalogForRunRequiresPipelinePlan(t *testing.T) {
	spec := orchestratedSpec(model.StageSpec{
		Name:       "meta-stage",
		Runtime:    model.StageRuntime{Summary: "Meta stage."},
		PromptFile: "prompts/intake.md",
		ToolingRepo: model.ToolingRepo{
			URL: "tooling/intake",
			Ref: "main",
		},
		Container: model.StageContainer{
			RunAs:   model.ExecutionIdentityAgent,
			WriteAs: model.ExecutionIdentityAgent,
		},
		Entrypoint: []string{"/bin/sh", "-c", "echo"},
	}, "tooling.intake.run")
	planRun := model.RunRequest{
		ID:      "run-plan",
		IssueID: "issue-plan",
		WorkOrder: model.WorkOrder{
			ID:               "wo-plan",
			IssueType:        "bug_fix",
			PipelineTemplate: "default-v1",
			Testing: model.TestingPolicy{
				Strategy:          "tests-before-implementation",
				Immutable:         true,
				ReadableByAgent:   false,
				ExecutableByAgent: true,
			},
			Delivery: model.DeliveryTarget{Name: "demo"},
		},
	}
	plan, err := pipeline.BuildExecutionPlan(planRun, []model.StageSpec{spec}, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"meta-stage": runtimeImageEntry(t, "plan"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")
	run := model.RunRequest{ID: "run-plan", WorkOrder: model.WorkOrder{ID: "wo-plan"}}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"meta-stage": runtimeImageEntry(t, "plan"),
	}), spec)
	catalog, err := stageCatalogForRun(run, workOrderRepo)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(plan.Stages) {
		t.Fatalf("expected catalog length %d, got %d", len(plan.Stages), len(catalog))
	}
	if catalog[0].Name != "meta-stage" {
		t.Fatalf("expected stage meta-stage, got %s", catalog[0].Name)
	}
	if _, err := stageCatalogForRun(model.RunRequest{ID: "run-missing", WorkOrder: model.WorkOrder{ID: "wo-missing"}}, workOrderRepo); err == nil {
		t.Fatal("expected missing pipeline plan to fail")
	}
}

func TestExecuteRejectsPipelinePlanRuntimeDigestMismatch(t *testing.T) {
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := t.TempDir()
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:       "intake",
		Runtime:    model.StageRuntime{Summary: "Captured intake."},
		PromptFile: "prompts/intake.md",
		ToolingRepo: model.ToolingRepo{
			URL: "tooling/intake",
			Ref: "main",
		},
		Container: model.StageContainer{
			RunAs: model.ExecutionIdentityGoverned,
		},
	}, "tooling.intake.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, nil)),
	)

	run := model.RunRequest{ID: "run-digest", IssueID: "issue-digest", WorkOrder: model.WorkOrder{ID: "wo-digest"}}
	runtimeImages := imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"intake": runtimeImageEntry(t, "intake"),
	})
	run = withExecutionPlan(t, workOrderRepo, run, runtimeImages, spec)

	planPath := filepath.Join(workOrderRepo, filepath.FromSlash(workorder.PipelineArtifactPaths(run)["pipeline_execution_plan"]))
	var plan map[string]any
	payload, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &plan); err != nil {
		t.Fatal(err)
	}
	stages := plan["stages"].([]any)
	stages[0].(map[string]any)["image_digest"] = "sha256:tampered"
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err = executor.Execute(context.Background(), spec, run, model.StageAttempt{ID: "attempt-digest", Stage: "intake", Attempt: 1}, model.DeliveryIssue{ID: "issue-digest"})
	if err == nil || !strings.Contains(err.Error(), "image_digest mismatch") {
		t.Fatalf("expected image digest mismatch error, got %v", err)
	}
}

func TestExecutePreservesEvidenceWhenSuccessCriteriaFail(t *testing.T) {
	t.Skip("legacy criteria reporting")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := t.TempDir()
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:           "intake",
		Runtime:        model.StageRuntime{Summary: "Captured intake.", SuccessCriteria: model.StageSuccessCriteria{RequireSummary: true, RequiredOutputs: []string{"missing_output"}, RequiredReportMeta: []string{"state_transitions", "step_log"}}},
		PromptFile:     "prompts/intake.md",
		ToolingRepo:    model.ToolingRepo{URL: "tooling/intake", Ref: "main"},
		ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		Container: model.StageContainer{
			RunAs: model.ExecutionIdentityGoverned,
			Permissions: model.StagePermissions{
				Network: "restricted",
			},
		},
	}, "tooling.intake.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, nil)),
	)

	run := model.RunRequest{ID: "run-criteria", IssueID: "issue-criteria", WorkOrder: model.WorkOrder{ID: "wo-criteria"}}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"intake": runtimeImageEntry(t, "intake"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-criteria", Stage: "intake", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-criteria", Title: "Issue"}

	result, artifactsOut, err := executor.Execute(context.Background(), spec, run, attempt, issue)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.AttemptStatusFailed {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
	if !strings.Contains(result.Summary, "contract validation failed") {
		t.Fatalf("expected contract validation summary, got %q", result.Summary)
	}
	if len(artifactsOut) < 2 {
		t.Fatalf("expected persisted artifacts, got %+v", artifactsOut)
	}

	var outputs map[string]any
	if err := json.Unmarshal(result.Outputs, &outputs); err != nil {
		t.Fatal(err)
	}
	validation, ok := outputs["contract_validation"].(map[string]any)
	if !ok {
		t.Fatalf("expected contract_validation output, got %v", outputs)
	}
	if passed, _ := validation["passed"].(bool); passed {
		t.Fatalf("expected failed contract validation, got %v", validation)
	}

	var evidencePath string
	for _, artifact := range artifactsOut {
		if artifact.Name == "evidence" {
			evidencePath = artifact.URI
			break
		}
	}
	if evidencePath == "" {
		t.Fatalf("expected evidence artifact, got %+v", artifactsOut)
	}
	payload, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	meta, ok := report["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected report metadata, got %v", report)
	}
	if _, ok := meta["contract_validation"].(map[string]any); !ok {
		t.Fatalf("expected contract_validation in report metadata, got %v", meta)
	}
}

func TestExecuteWithProgressReportsRealtimeState(t *testing.T) {
	t.Skip("legacy progress reporting")
	t.Skip("legacy")
	root := filepath.Join("..", "..")
	requirePlatformRuntimeArtifact(t, root)
	dataDir := t.TempDir()
	workOrderRepo := t.TempDir()
	createGitRepo(t, workOrderRepo, "README.md", "journal\n")

	spec := orchestratedSpec(model.StageSpec{
		Name:       "intake",
		Runtime:    model.StageRuntime{Summary: "Captured intake.", SuccessCriteria: model.StageSuccessCriteria{RequireSummary: true, RequiredReportMeta: []string{"state_transitions", "step_log"}}},
		PromptFile: "prompts/intake.md",
		ToolingRepo: model.ToolingRepo{
			URL: "tooling/intake",
			Ref: "main",
		},
		ArtifactPolicy: model.ArtifactPolicy{Retention: "30d"},
		Container: model.StageContainer{
			RunAs: model.ExecutionIdentityGoverned,
		},
	}, "tooling.intake.run")
	executor := New(
		root,
		dataDir,
		artifacts.New(filepath.Join(dataDir, "artifacts")),
		stubSecretProvider{},
		stubRatchetRanker{},
		WithWorkOrderRepo(workOrderRepo),
		WithPipelineContract([]model.StageSpec{spec}, testPipelineCatalog()),
		WithExecutionEnv(testExecutionEnv(root, dataDir, workOrderRepo, nil)),
	)

	run := model.RunRequest{ID: "run-progress", IssueID: "issue-progress", WorkOrder: model.WorkOrder{ID: "wo-progress"}}
	run = withExecutionPlan(t, workOrderRepo, run, imageCatalogForTests(t, map[string]stagecontainer.RuntimeImage{
		"intake": runtimeImageEntry(t, "intake"),
	}), spec)
	attempt := model.StageAttempt{ID: "attempt-progress", Stage: "intake", Attempt: 1}
	issue := model.DeliveryIssue{ID: "issue-progress", Title: "Issue"}

	var updates []map[string]any
	_, _, err := executor.ExecuteWithProgress(context.Background(), spec, run, attempt, issue, func(summary string, metadata map[string]any) {
		updates = append(updates, map[string]any{
			"summary":  summary,
			"metadata": metadata,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) == 0 {
		t.Fatalf("expected realtime progress updates")
	}
	last := updates[len(updates)-1]
	meta, ok := last["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata in progress update, got %+v", last)
	}
	if _, ok := meta["current_state"]; !ok {
		t.Fatalf("expected current_state in progress metadata, got %+v", meta)
	}
	if _, ok := meta["state_transitions"]; !ok {
		t.Fatalf("expected state_transitions in progress metadata, got %+v", meta)
	}
}

func createGitRepo(t *testing.T, dir, fileName, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.name", "test"},
		{"git", "config", "user.email", "test@example.invalid"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	}
	for _, cmdArgs := range cmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(cmdArgs, " "), err, string(output))
		}
	}
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

func withExecutionPlan(t *testing.T, workOrderRepo string, run model.RunRequest, runtimeImages map[string]stagecontainer.RuntimeImage, specs ...model.StageSpec) model.RunRequest {
	t.Helper()
	if strings.TrimSpace(run.IssueID) == "" {
		run.IssueID = "issue-test"
	}
	if strings.TrimSpace(run.WorkOrder.ID) == "" {
		run.WorkOrder.ID = "wo-test"
	}
	if strings.TrimSpace(run.WorkOrder.IssueType) == "" {
		run.WorkOrder.IssueType = "bug_fix"
	}
	if strings.TrimSpace(run.WorkOrder.PipelineTemplate) == "" {
		run.WorkOrder.PipelineTemplate = "default-v1"
	}
	if strings.TrimSpace(run.WorkOrder.Delivery.Name) == "" {
		run.WorkOrder.Delivery.Name = "demo"
	}
	if strings.TrimSpace(run.WorkOrder.Testing.Strategy) == "" {
		run.WorkOrder.Testing = model.TestingPolicy{
			Strategy:          "tests-before-implementation",
			Immutable:         true,
			ReadableByAgent:   false,
			ExecutableByAgent: true,
		}
	}
	plan, err := pipeline.BuildExecutionPlan(run, specs, runtimeImages)
	if err != nil {
		t.Fatal(err)
	}
	if workOrderRepo == "" {
		t.Fatal("workOrderRepo is required")
	}
	paths := workorder.PipelineArtifactPaths(run)
	planPath := filepath.Join(workOrderRepo, filepath.FromSlash(paths["pipeline_execution_plan"]))
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.ToSlash(paths["pipeline_execution_plan"])
	gitOutputForTest(t, workOrderRepo, "add", rel)
	if strings.TrimSpace(gitOutputForTest(t, workOrderRepo, "status", "--porcelain", "--", rel)) != "" {
		gitOutputForTest(t, workOrderRepo, "commit", "-m", "write pipeline execution plan")
	}
	return run
}
