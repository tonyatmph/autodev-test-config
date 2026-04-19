package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"


	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/artifacts"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/isolation"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/pipeline"
	"g7.mph.tech/mph-tech/autodev/internal/ratchet"
	repoplane "g7.mph.tech/mph-tech/autodev/internal/repos"
	"g7.mph.tech/mph-tech/autodev/internal/secrets"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
	"g7.mph.tech/mph-tech/autodev/internal/workorder"
)

type invariantRanker interface {
	RankedInvariants(context.Context, ratchet.RetrievalRequest) ([]ratchet.RankedInvariant, error)
}

type StageExecutor struct {
	artifactStore   *artifacts.Store
	rootDir         string
	dataDir         string
	secrets         secrets.Provider
	ratchets        invariantRanker
	repos           *repoplane.Manager
	repoRoots       []string
	workOrderRepo   string
	gitlab          gitlab.Adapter
	stageCatalog    []model.StageSpec
	pipelineCatalog map[string]any
	execEnv         *app.Env
}

type ProgressReporter func(summary string, metadata map[string]any)

type phaseTimer struct {
	last    time.Time
	timings []model.SubstageTiming
}

func (p *phaseTimer) Mark(name string) {
	now := time.Now().UTC()
	duration := now.Sub(p.last)
	if duration < 0 {
		duration = 0
	}
	p.timings = append(p.timings, model.SubstageTiming{Name: name, DurationMS: duration.Milliseconds()})
	p.last = now
}

func (p *phaseTimer) Finish() []model.SubstageTiming {
	return append([]model.SubstageTiming(nil), p.timings...)
}

type Option func(*StageExecutor)

func WithGitLab(adapter gitlab.Adapter) Option {
	return func(executor *StageExecutor) {
		executor.gitlab = adapter
	}
}

func WithRepoRoots(repoRoots []string) Option {
	return func(executor *StageExecutor) {
		executor.repoRoots = append([]string(nil), repoRoots...)
		executor.repos = repoplane.NewManager(executor.rootDir, executor.dataDir, repoRoots)
	}
}

func WithWorkOrderRepo(path string) Option {
	return func(executor *StageExecutor) {
		executor.workOrderRepo = path
	}
}

func WithPipelineContract(stageCatalog []model.StageSpec, pipelineCatalog map[string]any) Option {
	return func(executor *StageExecutor) {
		executor.stageCatalog = append([]model.StageSpec(nil), stageCatalog...)
		executor.pipelineCatalog = copyAnyMap(pipelineCatalog)
	}
}

func WithExecutionEnv(env app.Env) Option {
	return func(executor *StageExecutor) {
		copyEnv := env
		executor.execEnv = &copyEnv
	}
}

func New(rootDir, dataDir string, artifactStore *artifacts.Store, secretProvider secrets.Provider, ratchetRanker invariantRanker, opts ...Option) *StageExecutor {
	executor := &StageExecutor{
		artifactStore: artifactStore,
		rootDir:       rootDir,
		dataDir:       dataDir,
		secrets:       secretProvider,
		ratchets:      ratchetRanker,
		repos:         repoplane.NewManager(rootDir, dataDir, nil),
	}
	for _, opt := range opts {
		opt(executor)
	}
	return executor
}

type externalStageResult struct {
	Status      string         `json:"status"`
	Summary     string         `json:"summary"`
	Outputs     map[string]any `json:"outputs"`
	NextSignals []string       `json:"next_signals"`
}

type externalStageReport struct {
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id"`
	IssueID       string         `json:"issue_id"`
	Stage         string         `json:"stage"`
	Attempt       int            `json:"attempt"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	Outputs       map[string]any `json:"outputs"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
}

type contractValidation struct {
	Passed     bool      `json:"passed"`
	Violations []string  `json:"violations,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	Criteria   any       `json:"criteria,omitempty"`
}

func (e *StageExecutor) Execute(ctx context.Context, spec model.StageSpec, run model.RunRequest, attempt model.StageAttempt, issue model.DeliveryIssue) (model.StageResult, []model.ArtifactRef, error) {
	return e.ExecuteWithProgress(ctx, spec, run, attempt, issue, nil)
}

func (e *StageExecutor) ExecuteWithProgress(ctx context.Context, spec model.StageSpec, run model.RunRequest, attempt model.StageAttempt, issue model.DeliveryIssue, progress ProgressReporter) (model.StageResult, []model.ArtifactRef, error) {
	startedAt := time.Now().UTC()
	timer := phaseTimer{last: startedAt}
	workOrder := run.CanonicalWorkOrder()
	sandbox := isolation.New(e.rootDir, e.dataDir, e.repoRoots, spec, run)

	resolvedSecrets, err := secrets.ResolveAll(ctx, e.secrets, spec.AllowedSecrets)
	if err != nil {
		return model.StageResult{}, nil, fmt.Errorf("resolve stage secrets for %s: %w", spec.Name, err)
	}
	timer.Mark("resolve_secrets")

	if err := e.prepareWorkspace(sandbox, run.ID, spec, resolvedSecrets); err != nil {
		return model.StageResult{}, nil, err
	}
	timer.Mark("prepare_workspace")

	baseOutputs := map[string]any{
		"stage":                       spec.Name,
		"run_id":                      run.ID,
		"attempt_id":                  attempt.ID,
		"issue_id":                    issue.ID,
		"issue_title":                 issue.Title,
		"work_order":                  workOrder,
		"requested_outcome":           workOrder.RequestedOutcome,
		"application_repo":            workOrder.Delivery.ApplicationRepo,
		"selected_components":         workOrder.Delivery.SelectedComponentNames(),
		"ordered_selected_components": workOrder.Delivery.OrderedSelectedComponentNames(),
		"components":                  workOrder.Delivery.SelectedDeliveryComponents(),
		"environments":                workOrder.Delivery.Environments,
		"release":                     workOrder.Delivery.Release,
		"tooling_repo":                spec.ToolingRepo,
		"container":                   spec.ContainerConfig(),
		"success_criteria":            spec.SuccessCriteriaContract(),
		"timestamp":                   time.Now().UTC(),
		"resolved_secrets":            secretMetadata(resolvedSecrets),
		"runtime_isolation":           sandbox.Summary(),
	}
	if err := e.initializeStageRepos(ctx, spec, run, workOrder, sandbox, baseOutputs); err != nil {
		return model.StageResult{}, nil, err
	}

	invariantContext, err := e.loadInvariantContext(ctx, spec, run)
	if err != nil {
		return model.StageResult{}, nil, fmt.Errorf("load stage invariants for %s: %w", spec.Name, err)
	}
	baseOutputs["invariants"] = invariantContext
	timer.Mark("load_invariants")

	stageCatalog, err := stageCatalogForRun(run, e.workOrderRepo)
	if err != nil {
		return model.StageResult{}, nil, err
	}
	runtimeImages, err := stagecontainer.ResolveRuntimeImages(stageNamesFromSpecs(stageCatalog))
	if err != nil {
		return model.StageResult{}, nil, err
	}
	runtimeImage, err := runtimeImageForStage(run, spec.Name, e.workOrderRepo, runtimeImages)
	if err != nil {
		return model.StageResult{}, nil, err
	}

	workspace := filepath.Join(e.dataDir, "workspaces", run.ID, spec.Name)
	contextPayload := map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage": map[string]any{
			"name":             spec.Name,
			"operation":        spec.Operation,
			"operation_plan":   mustPlan(spec),
			"runtime":          spec.Runtime,
			"container":        spec.ContainerConfig(),
			"tooling_repo":     spec.ToolingRepo,
			"prompt_file":      spec.PromptFile,
			"success_criteria": spec.SuccessCriteriaContract(),
		},
		"run":                run,
		"attempt":            attempt,
		"issue":              issue,
		"work_order":         workOrder,
		"invariants":         invariantContext,
		"resolved_secrets":   secretMetadata(resolvedSecrets),
		"runtime_isolation":  sandbox.Summary(),
		"materialized_repos": baseOutputs["materialized_repos"],
		"pipeline_contract": map[string]any{
			"stage_catalog":    stageCatalog,
			"pipeline_catalog": copyAnyMap(e.pipelineCatalog),
			"runtime_images":   runtimeImagesForContract(runtimeImages),
		},
		"paths": map[string]any{
			"workspace":       workspace,
			"prompt":          filepath.Join(workspace, "prompt.md"),
			"tooling":         filepath.Join(workspace, "tooling"),
			"secrets_dir":     filepath.Join(workspace, "secrets"),
			"work_order_repo": e.workOrderRepo,
			"artifact_dir":    filepath.Join(e.dataDir, "artifacts"),
		},
	}
	if err := e.writeWorkspaceJSON(sandbox, run.ID, spec.Name, "invariants.json", invariantContext); err != nil {
		return model.StageResult{}, nil, fmt.Errorf("write invariant context: %w", err)
	}
	if err := e.writeWorkspaceJSON(sandbox, run.ID, spec.Name, "context.json", contextPayload); err != nil {
		return model.StageResult{}, nil, fmt.Errorf("write stage context: %w", err)
	}
	timer.Mark("write_contract_inputs")

	externalResult := externalStageResult{
		Status:      model.AttemptStatusSucceeded,
		Summary:     spec.SummaryText(),
		Outputs:     copyMap(baseOutputs),
		NextSignals: spec.NextStages(),
	}
	report := externalStageReport{
		SchemaVersion: "autodev-container-report-v1",
		RunID:         run.ID,
		IssueID:       issue.ID,
		Stage:         spec.Name,
		Attempt:       attempt.Attempt,
		Status:        externalResult.Status,
		Summary:       externalResult.Summary,
		Outputs:       copyMap(baseOutputs),
		Metadata:      map[string]any{},
		CreatedAt:     time.Now().UTC(),
	}

	runCtx := ctx
	var stopProgress context.CancelFunc
	var progressWG sync.WaitGroup
	if progress != nil {
		runCtx, stopProgress = context.WithCancel(ctx)
		progressWG.Add(1)
		go func() {
			defer progressWG.Done()
			e.streamStageState(runCtx, workspace, progress)
		}()
	}
	if err := e.runEntrypoint(runCtx, spec, run.ID, runtimeImage); err != nil {
		if stopProgress != nil {
			stopProgress()
			progressWG.Wait()
		}
		return model.StageResult{}, nil, fmt.Errorf("run stage entrypoint for %s: %w", spec.Name, err)
	}
	if stopProgress != nil {
		stopProgress()
		progressWG.Wait()
		e.emitLatestStageState(workspace, progress)
	}
	timer.Mark("run_container_contract")
	if err := e.loadWorkspaceJSON(run.ID, spec.Name, "result.json", &externalResult); err != nil {
		return model.StageResult{}, nil, err
	}
	if err := e.loadWorkspaceJSON(run.ID, spec.Name, "report.json", &report); err != nil {
		return model.StageResult{}, nil, err
	}

	timer.Mark("ingest_outputs")
	validation := validateSuccessCriteria(spec, externalResult, report)
	report.Metadata["contract_validation"] = validation
	externalResult.Outputs["stage_metadata"] = copyMap(report.Metadata)
	externalResult.Outputs["contract_validation"] = validation
	if !validation.Passed {
		externalResult.Status = model.AttemptStatusFailed
		externalResult.Summary = formatContractViolationSummary(validation.Violations)
		externalResult.NextSignals = spec.FailureStages()
	}
	outputBytes, err := json.MarshalIndent(externalResult.Outputs, "", "  ")
	if err != nil {
		return model.StageResult{}, nil, fmt.Errorf("encode stage outputs: %w", err)
	}
	stats := estimateAttemptStatsWithTimings(spec, attempt, externalResult.Outputs, outputBytes, startedAt, reportEvidenceCount(report), timer.Finish())
	timer.Mark("compute_stats")

	result := model.StageResult{
		Status:      strings.TrimSpace(externalResult.Status),
		Summary:     strings.TrimSpace(externalResult.Summary),
		Outputs:     outputBytes,
		NextSignals: append([]string(nil), externalResult.NextSignals...),
		Stats:       stats,
	}
	report.Status = result.Status
	report.Summary = result.Summary
	report.Outputs = externalResult.Outputs
	if err := e.writeWorkspaceJSON(sandbox, run.ID, spec.Name, "stats.json", stats); err != nil {
		return model.StageResult{}, nil, fmt.Errorf("write stage stats: %w", err)
	}

	artifactsOut := make([]model.ArtifactRef, 0, 4)
	executionArtifact, err := e.artifactStore.PutJSON(run.ID, spec.Name, "result", result, spec.ArtifactPolicy.Retention)
	if err != nil {
		return model.StageResult{}, nil, err
	}
	artifactsOut = append(artifactsOut, executionArtifact)

	evidenceArtifact, err := e.artifactStore.PutJSON(run.ID, spec.Name, "evidence", report, spec.ArtifactPolicy.Retention)
	if err != nil {
		return model.StageResult{}, nil, err
	}
	artifactsOut = append(artifactsOut, evidenceArtifact)

	statsArtifact, err := e.artifactStore.PutJSON(run.ID, spec.Name, "stats", stats, spec.ArtifactPolicy.Retention)
	if err != nil {
		return model.StageResult{}, nil, err
	}
	artifactsOut = append(artifactsOut, statsArtifact)

	for _, artifactSpec := range spec.Runtime.OutputArtifacts {
		payload, ok := outputArtifactPayload(externalResult.Outputs, artifactSpec.Source)
		if !ok {
			continue
		}
		artifact, err := e.artifactStore.PutJSON(run.ID, spec.Name, artifactSpec.Name, payload, spec.ArtifactPolicy.Retention)
		if err != nil {
			return model.StageResult{}, nil, err
		}
		artifactsOut = append(artifactsOut, artifact)
	}

	result.Stats.ArtifactCount = len(artifactsOut)
	stats.ArtifactCount = len(artifactsOut)
	return result, artifactsOut, nil
}

func (e *StageExecutor) streamStageState(ctx context.Context, workspace string, progress ProgressReporter) {
	if progress == nil {
		return
	}
	statePath := filepath.Join(workspace, "state.json")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	lastDigest := ""
	for {
		e.emitStageStateIfChanged(statePath, progress, &lastDigest)
		select {
		case <-ctx.Done():
			e.emitStageStateIfChanged(statePath, progress, &lastDigest)
			return
		case <-ticker.C:
		}
	}
}

func (e *StageExecutor) emitLatestStageState(workspace string, progress ProgressReporter) {
	if progress == nil {
		return
	}
	lastDigest := ""
	e.emitStageStateIfChanged(filepath.Join(workspace, "state.json"), progress, &lastDigest)
}

func (e *StageExecutor) emitStageStateIfChanged(path string, progress ProgressReporter, lastDigest *string) {
	payload, err := os.ReadFile(path)
	if err != nil || len(payload) == 0 {
		return
	}
	digest := string(payload)
	if lastDigest != nil && *lastDigest == digest {
		return
	}
	var state struct {
		Summary  string         `json:"summary"`
		Metadata map[string]any `json:"metadata"`
		State    string         `json:"state"`
		Step     string         `json:"current_step"`
		Status   string         `json:"status"`
	}
	if err := contracts.ReadFile(path, contracts.StageStateSchema, &state); err != nil {
		return
	}
	metadata := copyMap(state.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if strings.TrimSpace(state.State) != "" {
		metadata["current_state"] = state.State
	}
	if strings.TrimSpace(state.Step) != "" {
		metadata["current_step"] = state.Step
	}
	if strings.TrimSpace(state.Status) != "" {
		metadata["status"] = state.Status
	}
	summary := strings.TrimSpace(state.Summary)
	if summary == "" {
		parts := make([]string, 0, 2)
		if state.State != "" {
			parts = append(parts, "state="+state.State)
		}
		if state.Step != "" {
			parts = append(parts, "step="+state.Step)
		}
		summary = strings.Join(parts, " ")
	}
	progress(summary, metadata)
	if lastDigest != nil {
		*lastDigest = digest
	}
}

func validateSuccessCriteria(spec model.StageSpec, result externalStageResult, report externalStageReport) contractValidation {
	criteria := spec.SuccessCriteriaContract()
	if strings.TrimSpace(result.Status) == model.AttemptStatusBlocked {
		return contractValidation{
			Passed:    true,
			CheckedAt: time.Now().UTC(),
			Criteria:  criteria,
		}
	}
	violations := make([]string, 0)
	if expected := strings.TrimSpace(criteria.ResultStatus); expected != "" && strings.TrimSpace(result.Status) != expected {
		violations = append(violations, fmt.Sprintf("expected result status %q, got %q", expected, strings.TrimSpace(result.Status)))
	}
	if criteria.RequireSummary && strings.TrimSpace(result.Summary) == "" {
		violations = append(violations, "result summary is required but empty")
	}
	for _, key := range criteria.RequiredOutputs {
		if _, ok := result.Outputs[strings.TrimSpace(key)]; !ok {
			violations = append(violations, fmt.Sprintf("missing required output %q", strings.TrimSpace(key)))
		}
	}
	for _, key := range criteria.RequiredReportMeta {
		if _, ok := report.Metadata[strings.TrimSpace(key)]; !ok {
			violations = append(violations, fmt.Sprintf("missing required report metadata %q", strings.TrimSpace(key)))
		}
	}
	return contractValidation{
		Passed:     len(violations) == 0,
		Violations: violations,
		CheckedAt:  time.Now().UTC(),
		Criteria:   criteria,
	}
}

func formatContractViolationSummary(violations []string) string {
	if len(violations) == 0 {
		return "Stage contract validation failed."
	}
	return "Stage contract validation failed: " + strings.Join(violations, "; ")
}

func (e *StageExecutor) runEntrypoint(ctx context.Context, spec model.StageSpec, runID string, image stagecontainer.RuntimeImage) error {
	if len(spec.Entrypoint) == 0 {
		return fmt.Errorf("stage %s has no entrypoint configured", spec.Name)
	}
	if e.execEnv == nil {
		return fmt.Errorf("stage %s requires a container execution environment", spec.Name)
	}
	dockerRunner := &stagecontainer.Docker{}
	return dockerRunner.Run(ctx, stagecontainer.Config{
		Env: *e.execEnv,
	}, spec, runID, image)
}

func mustPlan(spec model.StageSpec) model.OperationPlan {
	plan, err := spec.OrchestrationPlan()
	if err != nil {
		panic(err)
	}
	return plan
}

func copyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	data, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func outputArtifactPayload(outputs map[string]any, source string) (any, bool) {
	switch strings.TrimSpace(source) {
	case "pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan":
		value, ok := outputs[strings.TrimSpace(source)]
		return value, ok
	case "manifest":
		if manifest, ok := outputs["release_manifest"]; ok {
			return manifest, true
		}
		if bundle, ok := outputs["release_bundle"].(map[string]any); ok {
			if manifest, ok := bundle["manifest"]; ok {
				return manifest, true
			}
		}
	}
	return nil, false
}

func copyAnyMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

func stageCatalogForRun(run model.RunRequest, workOrderRepo string) ([]model.StageSpec, error) {
	plan, err := pipelinePlanForRun(run, workOrderRepo)
	if err != nil {
		return nil, err
	}
	return pipeline.PlanSpecs(&plan), nil
}

func pipelinePlanForRun(run model.RunRequest, workOrderRepo string) (model.PipelineExecutionPlan, error) {
	if strings.TrimSpace(workOrderRepo) != "" {
		paths := workorder.PipelineArtifactPaths(run)
		if rel, ok := paths["pipeline_execution_plan"]; ok && rel != "" {
			var plan model.PipelineExecutionPlan
			path := filepath.Join(workOrderRepo, filepath.FromSlash(rel))
			if err := contracts.ReadFile(path, contracts.PipelineExecutionPlanSchema, &plan); err != nil {
				return model.PipelineExecutionPlan{}, fmt.Errorf("load pipeline execution plan for run %s: %w", run.ID, err)
			}
			if len(plan.Stages) == 0 {
				return model.PipelineExecutionPlan{}, fmt.Errorf("run %s pipeline execution plan has no stages", run.ID)
			}
			return plan, nil
		}
	}
	return model.PipelineExecutionPlan{}, fmt.Errorf("run %s missing materialized pipeline execution plan", run.ID)
}

func stageNamesFromSpecs(specs []model.StageSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) != "" {
			names = append(names, spec.Name)
		}
	}
	return names
}

func runtimeImageForStage(run model.RunRequest, stageName, workOrderRepo string, derived map[string]stagecontainer.RuntimeImage) (stagecontainer.RuntimeImage, error) {
	plan, err := pipelinePlanForRun(run, workOrderRepo)
	if err != nil {
		return stagecontainer.RuntimeImage{}, err
	}
	derivedImage, ok := derived[stageName]
	if !ok {
		var err error
		derivedImage, err = stagecontainer.ResolveRuntimeImage(stageName)
		if err != nil {
			return stagecontainer.RuntimeImage{}, fmt.Errorf("derive runtime image for stage %s: %w", stageName, err)
		}
	}
	for _, stage := range plan.Stages {
		if stage.Name != stageName {
			continue
		}
		if strings.TrimSpace(stage.Image) == "" || strings.TrimSpace(stage.ImageDigest) == "" {
			return stagecontainer.RuntimeImage{}, fmt.Errorf("run %s stage %s missing runtime image reference or digest", run.ID, stageName)
		}
		if stage.Image != derivedImage.Ref {
			return stagecontainer.RuntimeImage{}, fmt.Errorf("run %s stage %s image_ref mismatch: plan=%s derived=%s", run.ID, stageName, stage.Image, derivedImage.Ref)
		}
		if stage.ImageDigest != derivedImage.Digest {
			return stagecontainer.RuntimeImage{}, fmt.Errorf("run %s stage %s image_digest mismatch: plan=%s derived=%s", run.ID, stageName, stage.ImageDigest, derivedImage.Digest)
		}
		return derivedImage, nil
	}
	return stagecontainer.RuntimeImage{}, fmt.Errorf("run %s pipeline execution plan missing stage %s", run.ID, stageName)
}

func reportEvidenceCount(report externalStageReport) int {
	if len(report.Outputs) == 0 {
		return 0
	}
	if bundle, ok := report.Outputs["release_bundle"].(map[string]any); ok {
		if evidence, ok := bundle["evidence"].([]any); ok {
			return len(evidence)
		}
	}
	return len(report.Outputs)
}

func estimateAttemptStats(spec model.StageSpec, attempt model.StageAttempt, output map[string]any, outputBytes []byte, startedAt time.Time, evidenceCount int) model.AttemptStats {
	return estimateAttemptStatsWithTimings(spec, attempt, output, outputBytes, startedAt, evidenceCount, nil)
}

func estimateAttemptStatsWithTimings(spec model.StageSpec, attempt model.StageAttempt, output map[string]any, outputBytes []byte, startedAt time.Time, evidenceCount int, substages []model.SubstageTiming) model.AttemptStats {
	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}
	
	// Stats and cost logic has been moved to specialized containers.
	// The orchestrator only records basic mechanical execution metrics (duration).
	
	return model.AttemptStats{
		Stage:         spec.Name,
		WorkerID:      attempt.WorkerID,
		DurationMS:    duration.Milliseconds(),
		Substages:     substages,
		EvidenceCount: evidenceCount,
	}
}

func promptByteCount(output map[string]any) int {
	payload, err := json.Marshal(output)
	if err != nil {
		return 0
	}
	return len(payload)
}

func (e *StageExecutor) loadInvariantContext(ctx context.Context, spec model.StageSpec, run model.RunRequest) (map[string]any, error) {
	if e.ratchets == nil {
		return nil, fmt.Errorf("ratchet backend is required for stage %s", spec.Name)
	}
	ranked, err := e.ratchets.RankedInvariants(ctx, ratchet.RetrievalRequest{
		Stage:        spec.Name,
		RepoScope:    run.RepoScope(),
		Environment:  spec.Environment(),
		ServiceScope: run.ServiceScope(),
	})
	if err != nil {
		return nil, err
	}

	blocking := make([]ratchet.RankedInvariant, 0)
	advisories := make([]ratchet.RankedInvariant, 0, 5)
	for _, invariant := range ranked {
		if invariant.EnforcementMode == "block" {
			blocking = append(blocking, invariant)
			continue
		}
		if len(advisories) < 5 {
			advisories = append(advisories, invariant)
		}
	}

	return map[string]any{
		"stage":                    spec.Name,
		"scope":                    map[string]string{"repo": run.RepoScope(), "environment": spec.Environment(), "service": run.ServiceScope()},
		"blocking":                 blocking,
		"top_relevant":             advisories,
		"total_candidates":         len(ranked),
		"selection_strategy":       "global-invariants-with-stage-stats",
		"advisory_limit":           5,
		"blocking_always_included": true,
	}, nil
}

func emptyInvariantContext(spec model.StageSpec) map[string]any {
	return map[string]any{
		"stage":                    spec.Name,
		"blocking":                 []ratchet.RankedInvariant{},
		"top_relevant":             []ratchet.RankedInvariant{},
		"total_candidates":         0,
		"selection_strategy":       "global-invariants-with-stage-stats",
		"advisory_limit":           5,
		"blocking_always_included": true,
	}
}

func (e *StageExecutor) prepareWorkspace(sandbox *isolation.Sandbox, runID string, spec model.StageSpec, resolvedSecrets []secrets.Value) error {
	workspace := filepath.Join(e.dataDir, "workspaces", runID, spec.Name)
	return sandbox.MkdirAll(workspace, 0o755)
}

func (e *StageExecutor) writeWorkspaceJSON(sandbox *isolation.Sandbox, runID, stage, name string, payload any) error {
	workspace := filepath.Join(e.dataDir, "workspaces", runID, stage)
	if err := sandbox.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	data, err := contracts.Marshal(workspaceSchema(name), filepath.Join(workspace, name), payload)
	if err != nil {
		return err
	}
	return sandbox.WriteFile(filepath.Join(workspace, name), data, 0o644)
}


func sanitizeSecretFileName(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

func secretMetadata(values []secrets.Value) []map[string]string {
	out := make([]map[string]string, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]string{
			"name":   value.Name,
			"source": value.Source,
		})
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
