package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/config"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/locks"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/pipeline"
	"g7.mph.tech/mph-tech/autodev/internal/signals"
	"g7.mph.tech/mph-tech/autodev/internal/store"
	"g7.mph.tech/mph-tech/autodev/internal/workorder"
)

type Service struct {
	store         *store.Store
	gitlab        gitlab.Adapter
	locker        locks.Manager
	signals       signals.Emitter
	specs         map[string]model.StageSpec
	workOrderRepo string
}

func New(store *store.Store, gitlab gitlab.Adapter, locker locks.Manager, signaler signals.Emitter, specs []model.StageSpec, workOrderRepo string) *Service {
	if locker == nil {
		locker = locks.NoopManager{}
	}
	if signaler == nil {
		signaler = signals.NewService(signals.NoopStore{})
	}
	return &Service{
		store:         store,
		gitlab:        gitlab,
		locker:        locker,
		signals:       signaler,
		specs:         config.SpecMap(specs),
		workOrderRepo: workOrderRepo,
	}
}

func (s *Service) EnqueueFromGitLab() error {
	issues, err := s.gitlab.ListIssues()
	if err != nil {
		return err
	}

	return s.store.Save(func(state *model.PersistedState) error {
		for _, issue := range issues {
			tracked, ok := state.Issues[issue.ID]
			if ok {
				tracked.DeliveryIssue = issue
				state.Issues[issue.ID] = tracked
				continue
			}
			state.Issues[issue.ID] = model.TrackedIssue{DeliveryIssue: issue}
		}
		return nil
	})
}

func (s *Service) Reconcile() error {
	return s.reconcile("")
}

func (s *Service) ReconcileIssue(issueID string) error {
	return s.reconcile(issueID)
}

func (s *Service) reconcile(issueFilter string) error {
	return s.store.Save(func(state *model.PersistedState) error {
		for issueID, tracked := range state.Issues {
			if issueFilter != "" && issueID != issueFilter {
				continue
			}
			if !hasLabel(tracked.Labels, model.IssueLabelRequested) &&
				!hasLabel(tracked.Labels, model.IssueLabelActive) &&
				!hasLabel(tracked.Labels, model.IssueLabelAwaitingApproval) {
				continue
			}

			run := latestRunForIssue(*state, issueID)
			if run == nil {
				continue
			}

			byStage := store.AttemptsByStage(*state, run.ID)
			approved := tracked.Approval.Approved || hasLabel(tracked.Labels, "delivery/approved")
			plan, ok := s.pipelinePlanForRun(*run)
			if !ok {
				return fmt.Errorf("run %s missing materialized pipeline execution plan", run.ID)
			}
			eval := pipeline.Evaluate(&plan, *run, byStage, approved)
			for _, spec := range eval.Triggered {
				attemptID := store.NextID(state, "attempt")
				attempt := model.StageAttempt{
					ID:      attemptID,
					RunID:   run.ID,
					Stage:   spec.Name,
					Attempt: 1,
					Status:  model.AttemptStatusPending,
					Metadata: map[string]any{
						"queue_mode": string(spec.QueueMode()),
					},
				}
				state.Attempts[attemptID] = attempt
				byStage[spec.Name] = attempt
				if err := s.recordComment(state, issueID, fmt.Sprintf("Triggered stage `%s` queued as `%s`.", spec.Name, attemptID)); err != nil {
					return err
				}
			}
			for _, spec := range eval.Runnable {
				if store.RunningCount(*state, spec.Name) >= max(spec.MaxParallelism, 1) {
					continue
				}
				attemptID := store.NextID(state, "attempt")
				attempt := model.StageAttempt{
					ID:      attemptID,
					RunID:   run.ID,
					Stage:   spec.Name,
					Attempt: nextAttemptNumber(byStage, spec.Name),
					Status:  model.AttemptStatusPending,
					Metadata: map[string]any{
						"timeout_seconds": spec.TimeoutSeconds,
					},
				}
				state.Attempts[attemptID] = attempt
				byStage[spec.Name] = attempt
				if err := s.recordComment(state, issueID, fmt.Sprintf("Stage `%s` queued as `%s`.", spec.Name, attemptID)); err != nil {
					return err
				}
			}

			byStage = store.AttemptsByStage(*state, run.ID)
			runStatus := pipeline.Evaluate(&plan, *run, byStage, approved).Status
			run.Status = runStatus
			run.UpdatedAt = time.Now().UTC()
			state.Runs[run.ID] = *run

			switch runStatus {
			case model.RunStatusAwaitingApproval:
				if err := s.transitionIssue(state, tracked.DeliveryIssue, []string{model.IssueLabelAwaitingApproval}, "Run is awaiting production GitOps promotion approval."); err != nil {
					return err
				}
			case model.RunStatusCompleted:
				if err := s.transitionIssue(state, tracked.DeliveryIssue, []string{model.IssueLabelCompleted}, "Run completed successfully."); err != nil {
					return err
				}
			case model.RunStatusFailed:
				if err := s.transitionIssue(state, tracked.DeliveryIssue, []string{model.IssueLabelFailed}, "Run failed. Review stage artifacts and retry."); err != nil {
					return err
				}
			default:
				if err := s.transitionIssue(state, tracked.DeliveryIssue, []string{model.IssueLabelActive}, "Run remains active."); err != nil {
					return err
				}
			}
			if _, err := s.persistRunIndex(*run); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) EnsureRunForIssue(issueID string) (model.RunRequest, error) {
	var ensured model.RunRequest
	err := s.store.Save(func(state *model.PersistedState) error {
		tracked, ok := state.Issues[issueID]
		if !ok {
			return fmt.Errorf("issue %s is not tracked", issueID)
		}
		run := latestRunForIssue(*state, issueID)
		if run != nil && !runStatusTerminal(run.Status) {
			ensured = *run
			return nil
		}
		now := time.Now().UTC()
		runID := store.NextID(state, "run")
		workOrder := tracked.CanonicalWorkOrder()
		newRun := model.RunRequest{
			ID:               runID,
			IssueID:          issueID,
			WorkOrder:        workOrder,
			PipelineTemplate: workOrder.PipelineTemplate,
			Target:           workOrder.Delivery,
			Status:           model.RunStatusPending,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if commit, err := s.persistWorkOrder(newRun); err != nil {
			return err
		} else if commit != "" {
			newRun.Metadata = ensureMetadata(newRun.Metadata)
			newRun.Metadata["work_order_commit"] = commit
		}
		state.Runs[runID] = newRun
		ensured = newRun
		return nil
	})
	if err != nil {
		return model.RunRequest{}, err
	}
	return ensured, nil
}

func (s *Service) PersistPipelineArtifacts(runID string, outputs map[string]any) (model.RunRequest, error) {
	if len(outputs) == 0 {
		return model.RunRequest{}, fmt.Errorf("persist pipeline artifacts requires outputs")
	}
	var persisted model.RunRequest
	err := s.store.Save(func(state *model.PersistedState) error {
		run, ok := state.Runs[runID]
		if !ok {
			return fmt.Errorf("run %s not found", runID)
		}
		if s.workOrderRepo == "" {
			return fmt.Errorf("work order repo is required to persist pipeline artifacts")
		}
		if err := s.ensureWorkOrderRepo(); err != nil {
			return err
		}
		written, err := workorder.WritePipelineArtifacts(s.workOrderRepo, run, outputs)
		if err != nil {
			return err
		}
		if len(written) > 0 {
			addArgs := append([]string{"add"}, written...)
			if _, err := runGitCommand(s.workOrderRepo, addArgs...); err != nil {
				return fmt.Errorf("stage work-order git add: %w", err)
			}
			statusArgs := append([]string{"status", "--porcelain", "--"}, written...)
			status, err := runGitCommand(s.workOrderRepo, statusArgs...)
			if err != nil {
				return fmt.Errorf("pipeline artifact status: %w", err)
			}
			if strings.TrimSpace(status) != "" {
				if _, err := runGitCommand(s.workOrderRepo, "commit", "-m", fmt.Sprintf("run %s materialize pipeline", run.ID)); err != nil {
					return fmt.Errorf("commit pipeline artifacts: %w", err)
				}
			}
		}
		s.persistPipelinePlanEvidence(&run, outputs)
		run.UpdatedAt = time.Now().UTC()
		state.Runs[run.ID] = run
		if _, err := s.persistRunIndex(run); err != nil {
			return err
		}
		persisted = run
		return nil
	})
	if err != nil {
		return model.RunRequest{}, err
	}
	return persisted, nil
}

func (s *Service) ClaimNext(workerID string, lease time.Duration, allowedStages []string) (*model.StageAttempt, model.StageSpec, error) {
	return s.claimNext(workerID, lease, allowedStages, "")
}

func (s *Service) ClaimNextForIssue(workerID string, lease time.Duration, allowedStages []string, issueID string) (*model.StageAttempt, model.StageSpec, error) {
	return s.claimNext(workerID, lease, allowedStages, issueID)
}

func (s *Service) claimNext(workerID string, lease time.Duration, allowedStages []string, issueID string) (*model.StageAttempt, model.StageSpec, error) {
	var claimed *model.StageAttempt
	var spec model.StageSpec
	pendingSignals := make([]signals.PipelineEvent, 0, 1)

	err := s.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		attemptIDs := make([]string, 0, len(state.Attempts))
		for id, attempt := range state.Attempts {
			if attempt.Status == model.AttemptStatusPending {
				attemptIDs = append(attemptIDs, id)
			}
		}
		sort.Strings(attemptIDs)
		for _, id := range attemptIDs {
			attempt := state.Attempts[id]
			if len(allowedStages) > 0 && !slices.Contains(allowedStages, attempt.Stage) {
				continue
			}
			run := state.Runs[attempt.RunID]
			if issueID != "" && run.IssueID != issueID {
				continue
			}
			if run.Status == model.RunStatusFailed || run.Status == model.RunStatusCompleted {
				continue
			}
			plan, ok := s.pipelinePlanForRun(run)
			if !ok {
				return fmt.Errorf("run %s missing materialized pipeline execution plan", run.ID)
			}
			specMap := pipeline.PlanSpecsMap(&plan)
			stageSpec, ok := specMap[attempt.Stage]
			if !ok {
				return fmt.Errorf("run %s pipeline plan missing stage %s", run.ID, attempt.Stage)
			}
			lockKeys, err := s.acquireLocks(context.Background(), run, attempt, stageSpec, workerID, lease)
			if err != nil {
				return err
			}
			specLockKeys := locks.KeysForStage(run, stageSpec)
			if len(lockKeys) == 0 && len(specLockKeys) > 0 {
				pendingSignals = append(pendingSignals, signals.PipelineEvent{
					Kind:         "lock_contention",
					RunID:        run.ID,
					IssueID:      run.IssueID,
					AttemptID:    attempt.ID,
					Stage:        attempt.Stage,
					WorkerID:     workerID,
					RepoScope:    run.RepoScope(),
					Environment:  stageSpec.Environment(),
					ServiceScope: run.ServiceScope(),
					Severity:     "medium",
					Summary:      fmt.Sprintf("Stage %s could not acquire required locks.", attempt.Stage),
					Metadata:     map[string]any{"lock_keys": specLockKeys},
				})
				continue
			}
			expiry := now.Add(lease)
			attempt.Status = model.AttemptStatusRunning
			attempt.WorkerID = workerID
			attempt.StartedAt = &now
			attempt.LastHeartbeat = &now
			attempt.LeaseExpiresAt = &expiry
			if attempt.Metadata == nil {
				attempt.Metadata = map[string]any{}
			}
			if len(lockKeys) > 0 {
				attempt.Metadata["lock_keys"] = lockKeys
			}
			state.Attempts[id] = attempt
			copyAttempt := attempt
			claimed = &copyAttempt
			spec = stageSpec
			return s.recordComment(state, run.IssueID, fmt.Sprintf("Stage `%s` started on worker `%s`.", attempt.Stage, workerID))
		}
		return nil
	})
	if err != nil {
		return nil, model.StageSpec{}, err
	}
	for _, event := range pendingSignals {
		if err := s.emitSignals(event); err != nil {
			return nil, model.StageSpec{}, err
		}
	}
	if claimed == nil {
		return nil, model.StageSpec{}, nil
	}
	return claimed, spec, nil
}

func (s *Service) Heartbeat(attemptID string, lease time.Duration, summary string, metadata map[string]any) error {
	return s.store.Save(func(state *model.PersistedState) error {
		attempt, ok := state.Attempts[attemptID]
		if !ok {
			return fmt.Errorf("attempt %s not found", attemptID)
		}
		now := time.Now().UTC()
		expiry := now.Add(lease)
		attempt.LastHeartbeat = &now
		attempt.LeaseExpiresAt = &expiry
		if err := s.refreshLocks(context.Background(), attempt, lease); err != nil {
			return err
		}
		attempt.Metadata = ensureMetadata(attempt.Metadata)
		attempt.Metadata["runtime_summary"] = strings.TrimSpace(summary)
		if len(metadata) > 0 {
			attempt.Metadata["runtime_state"] = copyAnyMap(metadata)
		}
		state.Attempts[attemptID] = attempt
		run := state.Runs[attempt.RunID]
		run.Metadata = ensureMetadata(run.Metadata)
		stageStates, _ := run.Metadata["current_stage_states"].(map[string]any)
		if stageStates == nil {
			stageStates = map[string]any{}
		}
		stageState := map[string]any{
			"attempt_id":       attempt.ID,
			"attempt_number":   attempt.Attempt,
			"status":           attempt.Status,
			"summary":          strings.TrimSpace(summary),
			"last_heartbeat":   now,
			"lease_expires_at": expiry,
		}
		if len(metadata) > 0 {
			stageState["runtime_state"] = copyAnyMap(metadata)
		}
		stageStates[attempt.Stage] = stageState
		run.Metadata["current_stage_states"] = stageStates
		run.UpdatedAt = now
		state.Runs[run.ID] = run
		return nil
	})
}

func (s *Service) Complete(attemptID string, result model.StageResult, artifacts []model.ArtifactRef) error {
	var run model.RunRequest
	var event signals.PipelineEvent
	err := s.store.Save(func(state *model.PersistedState) error {
		attempt, ok := state.Attempts[attemptID]
		if !ok {
			return fmt.Errorf("attempt %s not found", attemptID)
		}
		now := time.Now().UTC()
		attempt.Result = &result
		attempt.Stats = result.Stats
		attempt.Artifacts = artifacts
		attempt.FinishedAt = &now
		attempt.LeaseExpiresAt = nil
		switch result.Status {
		case model.AttemptStatusSucceeded:
			attempt.Status = model.AttemptStatusSucceeded
		case model.AttemptStatusBlocked:
			attempt.Status = model.AttemptStatusBlocked
		default:
			attempt.Status = model.AttemptStatusFailed
		}
		if err := s.releaseLocks(context.Background(), attempt); err != nil {
			return err
		}
		var journalEntry map[string]any
		var outputs map[string]any
		if len(result.Outputs) > 0 {
			if err := json.Unmarshal(result.Outputs, &outputs); err == nil {
				journalEntry = s.extractJournalEntry(attempt.Stage, outputs)
			}
		}
		if journalEntry != nil {
			attempt.Metadata = ensureMetadata(attempt.Metadata)
			attempt.Metadata["journal_entry"] = journalEntry
		}
		run = state.Runs[attempt.RunID]
		if journalEntry != nil {
			run.Metadata = ensureMetadata(run.Metadata)
			run.Metadata["last_journal"] = journalEntry
			appendJournalHistory(run.Metadata, journalEntry)
		}
		if outputs != nil {
			s.persistPipelinePlanEvidence(&run, outputs)
			s.persistStageCommits(&attempt, &run, outputs)
		}
		attempt.Metadata = ensureMetadata(attempt.Metadata)
		delete(attempt.Metadata, "runtime_state")
		delete(attempt.Metadata, "runtime_summary")
		run.Metadata = ensureMetadata(run.Metadata)
		stageStates, _ := run.Metadata["current_stage_states"].(map[string]any)
		if stageStates == nil {
			stageStates = map[string]any{}
		}
		stageState := map[string]any{
			"attempt_id":       attempt.ID,
			"attempt_number":   attempt.Attempt,
			"status":           attempt.Status,
			"summary":          result.Summary,
			"finished_at":      now,
			"last_heartbeat":   attempt.LastHeartbeat,
			"lease_expires_at": attempt.LeaseExpiresAt,
		}
		if outputs != nil {
			if reportMeta := extractReportMetadata(outputs); len(reportMeta) > 0 {
				stageState["runtime_state"] = reportMeta
			}
		}
		stageStates[attempt.Stage] = stageState
		run.Metadata["current_stage_states"] = stageStates
		stageJournalEntry, err := s.persistStageReport(run, attempt, result, artifacts)
		if err != nil {
			return err
		}
		if stageJournalEntry != nil {
			attempt.Metadata = ensureMetadata(attempt.Metadata)
			attempt.Metadata["stage_report"] = stageJournalEntry
			run.Metadata = ensureMetadata(run.Metadata)
			run.Metadata["run_index"] = stageJournalEntry["index"]
		}
		state.Attempts[attemptID] = attempt
		run.Stats = collectRunStats(*state, run.ID)
		run.UpdatedAt = now
		state.Runs[run.ID] = run
		spec := s.specs[attempt.Stage]
		event = signals.PipelineEvent{
			Kind:         "stage_completed",
			RunID:        run.ID,
			IssueID:      run.IssueID,
			AttemptID:    attempt.ID,
			Stage:        attempt.Stage,
			WorkerID:     attempt.WorkerID,
			RepoScope:    run.RepoScope(),
			Environment:  spec.Environment(),
			ServiceScope: run.ServiceScope(),
			Status:       attempt.Status,
			Severity:     severityForAttempt(attempt.Status, spec),
			Summary:      result.Summary,
			DurationMS:   result.Stats.DurationMS,
			CostUSD:      result.Stats.Cost.TotalUSD,
			Metadata: map[string]any{
				"artifact_count": len(artifacts),
				"substages":      result.Stats.Substages,
			},
		}

		issueID := run.IssueID
		summary := fmt.Sprintf("Stage `%s` completed with status `%s`: %s Cost: $%.6f. Duration: %dms.", attempt.Stage, attempt.Status, result.Summary, result.Stats.Cost.TotalUSD, result.Stats.DurationMS)
		if err := s.recordComment(state, issueID, summary); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if event.Kind != "" {
		if err := s.emitSignals(event); err != nil {
			return err
		}
	}
	_ = run
	return nil
}

func (s *Service) RecoverStuckRuns(maxAge time.Duration) error {
	var pendingSignals []signals.PipelineEvent
	err := s.store.Save(func(state *model.PersistedState) error {
		now := time.Now().UTC()
		for id, attempt := range state.Attempts {
			if attempt.Status != model.AttemptStatusRunning {
				continue
			}
			if attempt.LeaseExpiresAt == nil || attempt.LeaseExpiresAt.Add(maxAge).After(now) {
				continue
			}
			attempt.Status = model.AttemptStatusPending
			attempt.WorkerID = ""
			attempt.LeaseExpiresAt = nil
			if err := s.releaseLocks(context.Background(), attempt); err != nil {
				return err
			}
			state.Attempts[id] = attempt
			run := state.Runs[attempt.RunID]
			spec := s.specs[attempt.Stage]
			pendingSignals = append(pendingSignals, signals.PipelineEvent{
				Kind:         "stale_attempt_recovered",
				RunID:        run.ID,
				IssueID:      run.IssueID,
				AttemptID:    attempt.ID,
				Stage:        attempt.Stage,
				RepoScope:    run.RepoScope(),
				Environment:  spec.Environment(),
				ServiceScope: run.ServiceScope(),
				Severity:     "low",
				Summary:      fmt.Sprintf("Recovered stale stage %s back to pending.", attempt.Stage),
			})
			if err := s.recordComment(state, run.IssueID, fmt.Sprintf("Recovered stale stage `%s` back to pending.", attempt.Stage)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, event := range pendingSignals {
		if err := s.emitSignals(event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) acquireLocks(ctx context.Context, run model.RunRequest, attempt model.StageAttempt, spec model.StageSpec, workerID string, lease time.Duration) ([]string, error) {
	keys := locks.KeysForStage(run, spec)
	if len(keys) == 0 {
		return nil, nil
	}
	acquired := make([]string, 0, len(keys))
	for _, key := range keys {
		ok, err := s.locker.TryAcquire(ctx, key, attempt.ID, lease, locks.Metadata{
			RunID:     run.ID,
			AttemptID: attempt.ID,
			IssueID:   run.IssueID,
			Stage:     attempt.Stage,
			WorkerID:  workerID,
		})
		if err != nil {
			s.releaseAcquired(ctx, acquired, attempt.ID)
			return nil, err
		}
		if !ok {
			s.releaseAcquired(ctx, acquired, attempt.ID)
			return nil, nil
		}
		acquired = append(acquired, key)
	}
	return acquired, nil
}

func (s *Service) refreshLocks(ctx context.Context, attempt model.StageAttempt, lease time.Duration) error {
	for _, key := range lockKeys(attempt) {
		if err := s.locker.Refresh(ctx, key, attempt.ID, lease); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) releaseLocks(ctx context.Context, attempt model.StageAttempt) error {
	for _, key := range lockKeys(attempt) {
		if err := s.locker.Release(ctx, key, attempt.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) releaseAcquired(ctx context.Context, keys []string, owner string) {
	for _, key := range keys {
		_ = s.locker.Release(ctx, key, owner)
	}
}

func lockKeys(attempt model.StageAttempt) []string {
	raw, ok := attempt.Metadata["lock_keys"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var keys []string
	if err := json.Unmarshal(encoded, &keys); err != nil {
		return nil
	}
	return keys
}

func ensureMetadata(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func copyAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

func appendJournalHistory(metadata map[string]any, entry map[string]any) {
	const key = "journal_history"
	history, _ := metadata[key].([]any)
	metadata[key] = append(history, entry)
}

func (s *Service) extractJournalEntry(stage string, outputs map[string]any) map[string]any {
	if outputs == nil {
		return nil
	}
	entry := map[string]any{"stage": stage}
	if components := collectImplementationCommits(outputs); len(components) > 0 {
		entry["component_commits"] = components
	}
	if plan := collectPromotionPlan(outputs); len(plan) > 0 {
		entry["promotion_plan"] = plan
	}
	if manifest := collectReleaseManifestSummary(outputs); len(manifest) > 0 {
		entry["release_manifest"] = manifest
	}
	if journal, ok := outputs["journal"].(map[string]any); ok && len(journal) > 0 {
		entry["journal"] = journal
	}
	if len(entry) == 1 {
		return nil
	}
	return entry
}

func collectImplementationCommits(outputs map[string]any) []map[string]any {
	bundle, ok := outputs["implementation_bundle"].(map[string]any)
	if !ok {
		return nil
	}
	compsRaw, ok := bundle["components"].([]any)
	if !ok {
		return nil
	}
	commits := make([]map[string]any, 0, len(compsRaw))
	for _, raw := range compsRaw {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := comp["name"].(string)
		if name == "" {
			continue
		}
		info := map[string]any{"component": name}
		if branch, _ := comp["branch_name"].(string); branch != "" {
			info["branch"] = branch
		}
		if repoPath, _ := comp["project_path"].(string); repoPath != "" {
			info["repo"] = repoPath
		}
		if repoStatus, _ := comp["repo_status"].(string); repoStatus != "" {
			info["repo_status"] = repoStatus
		}
		if outcome, ok := comp["mutation_outcome"].(map[string]any); ok {
			if commit, _ := outcome["commit_sha"].(string); commit != "" {
				info["commit_sha"] = commit
			}
			if mutated, ok := outcome["mutated"].(bool); ok {
				info["mutated"] = mutated
			}
		}
		if len(info) > 1 {
			commits = append(commits, info)
		}
	}
	return commits
}

func extractReportMetadata(outputs map[string]any) map[string]any {
	metadata, _ := outputs["stage_metadata"].(map[string]any)
	metadata = copyAnyMap(metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if report, ok := outputs["contract_validation"]; ok {
		metadata["contract_validation"] = report
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func collectPromotionPlan(outputs map[string]any) map[string]any {
	planRaw, ok := outputs["promotion_plan"].(map[string]any)
	if !ok {
		return nil
	}
	plan := map[string]any{}
	if environment, _ := planRaw["environment"].(string); environment != "" {
		plan["environment"] = environment
	}
	if branch, _ := planRaw["branch"].(string); branch != "" {
		plan["branch"] = branch
	}
	if ready, ok := planRaw["ready"].(bool); ok {
		plan["ready"] = ready
	}
	if summary, _ := planRaw["summary"].(string); summary != "" {
		plan["summary"] = summary
	}
	if gitops, ok := planRaw["gitops_repo"].(map[string]any); ok && len(gitops) > 0 {
		plan["gitops_repo"] = gitops
	}
	if steps, ok := planRaw["steps"].([]any); ok && len(steps) > 0 {
		plan["steps"] = steps
	}
	if issues, ok := planRaw["issues"].([]any); ok && len(issues) > 0 {
		plan["issues"] = issues
	}
	if len(plan) == 0 {
		return nil
	}
	return plan
}

func collectReleaseManifestSummary(outputs map[string]any) map[string]any {
	bundle, ok := outputs["release_bundle"].(map[string]any)
	if !ok {
		return nil
	}
	summary := map[string]any{}
	if manifest, ok := bundle["manifest"].(map[string]any); ok {
		if runID, _ := manifest["run_id"].(string); runID != "" {
			summary["run_id"] = runID
		}
		if issueID, _ := manifest["issue_id"].(string); issueID != "" {
			summary["issue_id"] = issueID
		}
		if promotions, ok := manifest["promotions"].([]any); ok && len(promotions) > 0 {
			summary["promotions"] = promotions
		}
	}
	if evidence, ok := bundle["evidence"].([]any); ok && len(evidence) > 0 {
		summary["evidence"] = evidence
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func (s *Service) persistWorkOrder(run model.RunRequest) (string, error) {
	if s.workOrderRepo == "" {
		return "", nil
	}
	if err := s.ensureWorkOrderRepo(); err != nil {
		return "", err
	}
	file := filepath.Join(s.workOrderRepo, workOrderRelativePath(run))
	if err := contracts.WriteFile(file, "", run.WorkOrder); err != nil {
		return "", fmt.Errorf("write work order: %w", err)
	}
	rel := filepath.ToSlash(mustRel(s.workOrderRepo, file))
	if _, err := runGitCommand(s.workOrderRepo, "add", rel); err != nil {
		return "", fmt.Errorf("stage work order: %w", err)
	}
	status, err := runGitCommand(s.workOrderRepo, "status", "--porcelain", "--", rel)
	if err != nil {
		return "", fmt.Errorf("work order status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		if _, err := runGitCommand(s.workOrderRepo, "commit", "-m", fmt.Sprintf("work order %s", run.ID)); err != nil {
			return "", fmt.Errorf("commit work order: %w", err)
		}
	}
	sha, err := runGitCommand(s.workOrderRepo, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("work order HEAD: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

func (s *Service) persistStageReport(run model.RunRequest, attempt model.StageAttempt, result model.StageResult, artifacts []model.ArtifactRef) (map[string]any, error) {
	if s.workOrderRepo == "" {
		return nil, nil
	}
	if err := s.ensureWorkOrderRepo(); err != nil {
		return nil, err
	}
	workOrderPath := filepath.Join(s.workOrderRepo, workOrderRelativePath(run))
	runDir := filepath.Join(s.workOrderRepo, runRelativeDir(run))
	attemptDir := filepath.Join(runDir, "stages", attempt.Stage, fmt.Sprintf("attempt-%02d", attempt.Attempt))
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return nil, fmt.Errorf("create stage report dir: %w", err)
	}

	decodedOutputs := decodeOutputs(result.Outputs)
	summary := map[string]any{
		"run_id":        run.ID,
		"issue_id":      run.IssueID,
		"work_order_id": canonicalWorkOrderID(run),
		"stage":         attempt.Stage,
		"attempt":       attempt.Attempt,
		"status":        attempt.Status,
		"summary":       result.Summary,
		"finished_at":   attempt.FinishedAt,
	}
	report := model.StageReportSchema{
		SchemaVersion: model.StageReportSchemaVersion,
		RunID:         run.ID,
		IssueID:       run.IssueID,
		Stage:         attempt.Stage,
		Attempt:       attempt.Attempt,
		Status:        attempt.Status,
		Summary:       result.Summary,
		Stats:         result.Stats,
		Artifacts:     artifacts,
		Outputs:       decodedOutputs,
		Metadata: map[string]any{
			"worker_id":     attempt.WorkerID,
			"work_order_id": canonicalWorkOrderID(run),
		},
		StartedAt:  attempt.StartedAt,
		FinishedAt: attempt.FinishedAt,
		CreatedAt:  time.Now().UTC(),
	}
	summaryPath := filepath.Join(attemptDir, "summary.json")
	reportPath := filepath.Join(attemptDir, "report.json")
	if err := writePrettyJSON(summaryPath, summary); err != nil {
		return nil, err
	}
	if err := writePrettyJSON(reportPath, report); err != nil {
		return nil, err
	}
	planFiles := []string(nil)
	if outputMap, ok := decodedOutputs.(map[string]any); ok {
		var err error
		planFiles, err = workorder.WritePipelineArtifacts(s.workOrderRepo, run, outputMap)
		if err != nil {
			return nil, err
		}
	}

	indexPath := filepath.Join(runDir, "index.json")
	index, err := s.buildRunIndex(run, workOrderPath, runDir)
	if err != nil {
		return nil, err
	}
	if err := writePrettyJSON(indexPath, index); err != nil {
		return nil, err
	}

	relSummary := filepath.ToSlash(mustRel(s.workOrderRepo, summaryPath))
	relReport := filepath.ToSlash(mustRel(s.workOrderRepo, reportPath))
	relIndex := filepath.ToSlash(mustRel(s.workOrderRepo, indexPath))
	addArgs := []string{"add", relSummary, relReport, relIndex}
	addArgs = append(addArgs, planFiles...)
	if _, err := runGitCommand(s.workOrderRepo, addArgs...); err != nil {
		return nil, fmt.Errorf("stage work-order git add: %w", err)
	}
	statusArgs := []string{"status", "--porcelain", "--", relSummary, relReport, relIndex}
	statusArgs = append(statusArgs, planFiles...)
	status, err := runGitCommand(s.workOrderRepo, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("stage work-order status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		if _, err := runGitCommand(s.workOrderRepo, "commit", "-m", fmt.Sprintf("run %s stage %s attempt %02d", run.ID, attempt.Stage, attempt.Attempt)); err != nil {
			return nil, fmt.Errorf("commit stage report: %w", err)
		}
	}
	sha, err := runGitCommand(s.workOrderRepo, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("stage work-order HEAD: %w", err)
	}
	return map[string]any{
		"repository":     filepath.Base(s.workOrderRepo),
		"work_order":     filepath.ToSlash(mustRel(s.workOrderRepo, workOrderPath)),
		"index":          relIndex,
		"summary":        relSummary,
		"report":         relReport,
		"commit_sha":     strings.TrimSpace(sha),
		"stage":          attempt.Stage,
		"attempt_number": attempt.Attempt,
	}, nil
}

func (s *Service) persistStageCommits(attempt *model.StageAttempt, run *model.RunRequest, outputs map[string]any) {
	generatorCommit := stageCommitFromOutputs(outputs, "generator_commit", "generator_journal", "commit_sha")
	if generatorCommit != "" {
		attempt.Metadata = ensureMetadata(attempt.Metadata)
		attempt.Metadata["generator_commit"] = generatorCommit
		run.Metadata = ensureMetadata(run.Metadata)
		appendMetadataList(run.Metadata, "generator_commits", generatorCommit)
	}
	promotionCommit := stageCommitFromOutputs(outputs, "promotion_gitops_commit", "promotion_commit", "gitops_commit")
	if promotionCommit != "" {
		attempt.Metadata = ensureMetadata(attempt.Metadata)
		attempt.Metadata["promotion_gitops_commit"] = promotionCommit
		run.Metadata = ensureMetadata(run.Metadata)
		appendMetadataList(run.Metadata, "promotion_commits", promotionCommit)
	}
}

func (s *Service) persistPipelinePlanEvidence(run *model.RunRequest, outputs map[string]any) {
	if run == nil || outputs == nil {
		return
	}
	for _, key := range []string{"pipeline_intent", "policy_evaluation", "pipeline_build_plan", "pipeline_execution_plan"} {
		value, ok := outputs[key]
		if !ok {
			continue
		}
		run.Metadata = ensureMetadata(run.Metadata)
		run.Metadata[key] = value
	}
}

func (s *Service) pipelinePlanForRun(run model.RunRequest) (model.PipelineExecutionPlan, bool) {
	return s.pipelinePlanFromJournal(run)
}

func (s *Service) pipelinePlanFromJournal(run model.RunRequest) (model.PipelineExecutionPlan, bool) {
	if s.workOrderRepo == "" {
		return model.PipelineExecutionPlan{}, false
	}
	paths := workorder.PipelineArtifactPaths(run)
	rel, ok := paths["pipeline_execution_plan"]
	if !ok || rel == "" {
		return model.PipelineExecutionPlan{}, false
	}
	abs := filepath.Join(s.workOrderRepo, filepath.FromSlash(rel))
	var plan model.PipelineExecutionPlan
	if err := contracts.ReadFile(abs, contracts.PipelineExecutionPlanSchema, &plan); err != nil {
		return model.PipelineExecutionPlan{}, false
	}
	if len(plan.Stages) == 0 {
		return model.PipelineExecutionPlan{}, false
	}
	return plan, true
}

func appendMetadataList(metadata map[string]any, key, value string) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	list, _ := metadata[key].([]any)
	metadata[key] = append(list, value)
}

func stringFromMap(outputs map[string]any, key string) string {
	if outputs == nil {
		return ""
	}
	if v, ok := outputs[key]; ok {
		if str, ok := v.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func stageCommitFromOutputs(outputs map[string]any, scalarKey, nestedKey, nestedField string) string {
	if commit := stringFromMap(outputs, scalarKey); commit != "" {
		return commit
	}
	if outputs == nil {
		return ""
	}
	raw, ok := outputs[nestedKey]
	if !ok {
		return ""
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := nested[nestedField].(string)
	return strings.TrimSpace(value)
}

func (s *Service) ensureWorkOrderRepo() error {
	if s.workOrderRepo == "" {
		return nil
	}
	if err := os.MkdirAll(s.workOrderRepo, 0o755); err != nil {
		return fmt.Errorf("create work order dir: %w", err)
	}
	gitDir := filepath.Join(s.workOrderRepo, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat work order git dir: %w", err)
	}
	if _, err := runGitCommand(s.workOrderRepo, "init", "--initial-branch=main"); err != nil {
		return fmt.Errorf("init work order repo: %w", err)
	}
	if _, err := runGitCommand(s.workOrderRepo, "config", "user.name", "autodev-workorder"); err != nil {
		return fmt.Errorf("configure work order user: %w", err)
	}
	if _, err := runGitCommand(s.workOrderRepo, "config", "user.email", "autodev.workorder@example.com"); err != nil {
		return fmt.Errorf("configure work order email: %w", err)
	}
	readme := filepath.Join(s.workOrderRepo, "README.md")
	if err := os.WriteFile(readme, []byte("# Work-order journal\n\nWork orders are recorded here for auditing.\n"), 0o644); err != nil {
		return fmt.Errorf("write work order readme: %w", err)
	}
	if _, err := runGitCommand(s.workOrderRepo, "add", "README.md"); err != nil {
		return fmt.Errorf("stage work order readme: %w", err)
	}
	if _, err := runGitCommand(s.workOrderRepo, "commit", "-m", "Initialize work-order journal"); err != nil {
		return fmt.Errorf("commit work order readme: %w", err)
	}
	return nil
}

func (s *Service) buildRunIndex(run model.RunRequest, workOrderPath, runDir string) (map[string]any, error) {
	index := map[string]any{
		"run_id":             run.ID,
		"issue_id":           run.IssueID,
		"work_order_id":      canonicalWorkOrderID(run),
		"work_order":         filepath.ToSlash(mustRel(s.workOrderRepo, workOrderPath)),
		"components":         run.DeliveryTarget().OrderedSelectedComponentNames(),
		"run_status":         run.Status,
		"updated_at":         time.Now().UTC(),
		"stage_reports":      map[string]any{},
		"release":            run.Metadata["last_journal"],
		"generator_commits":  run.Metadata["generator_commits"],
		"promotion_commits":  run.Metadata["promotion_commits"],
		"pipeline_artifacts": workorder.PipelineArtifactPaths(run),
	}
	stageReports := index["stage_reports"].(map[string]any)
	stageRoot := filepath.Join(runDir, "stages")
	entries, err := os.ReadDir(stageRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read stage report dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stageName := entry.Name()
		attemptDirs, err := os.ReadDir(filepath.Join(stageRoot, stageName))
		if err != nil {
			return nil, fmt.Errorf("read stage attempts for %s: %w", stageName, err)
		}
		attemptPaths := make([]map[string]any, 0, len(attemptDirs))
		for _, attemptDir := range attemptDirs {
			if !attemptDir.IsDir() {
				continue
			}
			base := filepath.Join(stageRoot, stageName, attemptDir.Name())
			attemptPaths = append(attemptPaths, map[string]any{
				"attempt_dir": attemptDir.Name(),
				"summary":     filepath.ToSlash(mustRel(s.workOrderRepo, filepath.Join(base, "summary.json"))),
				"report":      filepath.ToSlash(mustRel(s.workOrderRepo, filepath.Join(base, "report.json"))),
			})
		}
		sort.Slice(attemptPaths, func(i, j int) bool {
			return attemptPaths[i]["attempt_dir"].(string) < attemptPaths[j]["attempt_dir"].(string)
		})
		stageReports[stageName] = attemptPaths
	}
	return index, nil
}

func (s *Service) persistRunIndex(run model.RunRequest) (string, error) {
	if s.workOrderRepo == "" {
		return "", nil
	}
	if err := s.ensureWorkOrderRepo(); err != nil {
		return "", err
	}
	workOrderPath := filepath.Join(s.workOrderRepo, workOrderRelativePath(run))
	runDir := filepath.Join(s.workOrderRepo, runRelativeDir(run))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("create run index dir: %w", err)
	}
	index, err := s.buildRunIndex(run, workOrderPath, runDir)
	if err != nil {
		return "", err
	}
	indexPath := filepath.Join(runDir, "index.json")
	if err := writePrettyJSON(indexPath, index); err != nil {
		return "", err
	}
	relIndex := filepath.ToSlash(mustRel(s.workOrderRepo, indexPath))
	if _, err := runGitCommand(s.workOrderRepo, "add", relIndex); err != nil {
		return "", fmt.Errorf("stage run index add: %w", err)
	}
	status, err := runGitCommand(s.workOrderRepo, "status", "--porcelain", "--", relIndex)
	if err != nil {
		return "", fmt.Errorf("stage run index status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		if _, err := runGitCommand(s.workOrderRepo, "commit", "-m", fmt.Sprintf("run %s index update", run.ID)); err != nil {
			return "", fmt.Errorf("commit run index: %w", err)
		}
	}
	sha, err := runGitCommand(s.workOrderRepo, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("run index HEAD: %w", err)
	}
	return strings.TrimSpace(sha), nil
}

func canonicalWorkOrderID(run model.RunRequest) string {
	workOrder := run.CanonicalWorkOrder()
	if strings.TrimSpace(workOrder.ID) != "" {
		return sanitizePathSegment(workOrder.ID)
	}
	return sanitizePathSegment(run.ID)
}

func workOrderRelativePath(run model.RunRequest) string {
	return filepath.Join("work-orders", canonicalWorkOrderID(run), "work-order.json")
}

func runRelativeDir(run model.RunRequest) string {
	return filepath.Join("work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID))
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "..", "-")
	return replacer.Replace(value)
}

func decodeOutputs(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return decoded
	}
	return map[string]any{"raw": string(raw)}
}

func writePrettyJSON(path string, value any) error {
	if err := contracts.WriteFile(path, journalSchema(path), value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func journalSchema(path string) string {
	switch filepath.Base(path) {
	case "report.json":
		return contracts.StageReportSchema
	default:
		return ""
	}
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (s *Service) Snapshot() (model.PersistedState, error) {
	return s.store.Load()
}

func (s *Service) recordComment(state *model.PersistedState, issueID, body string) error {
	issue := state.Issues[issueID]
	comment := model.IssueComment{
		ID:        store.NextID(state, "comment"),
		IssueID:   issueID,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	issue.Comments = append(issue.Comments, comment)
	state.Issues[issueID] = issue
	return s.gitlab.AppendComment(issueID, body)
}

func (s *Service) transitionIssue(state *model.PersistedState, issue model.DeliveryIssue, labels []string, comment string) error {
	updated := issue
	updated.Labels = normalizeLabels(issue.Labels, labels)
	state.Issues[issue.ID] = model.TrackedIssue{
		DeliveryIssue: updated,
		Comments:      state.Issues[issue.ID].Comments,
	}
	if err := s.gitlab.SetLabels(issue.ID, updated.Labels); err != nil {
		return err
	}
	return s.recordComment(state, issue.ID, comment)
}

func latestRunForIssue(state model.PersistedState, issueID string) *model.RunRequest {
	var latest *model.RunRequest
	for _, run := range state.Runs {
		if run.IssueID != issueID {
			continue
		}
		copyRun := run
		if latest == nil || copyRun.CreatedAt.After(latest.CreatedAt) {
			latest = &copyRun
		}
	}
	return latest
}

func runStatusTerminal(status string) bool {
	switch status {
	case model.RunStatusAwaitingApproval, model.RunStatusCompleted, model.RunStatusFailed:
		return true
	default:
		return false
	}
}

func nextAttemptNumber(byStage map[string]model.StageAttempt, stage string) int {
	if existing, ok := byStage[stage]; ok {
		return existing.Attempt + 1
	}
	return 1
}

func collectRunStats(state model.PersistedState, runID string) model.RunStats {
	stageIndex := make(map[string]*model.StageTotals)
	for _, attempt := range state.Attempts {
		if attempt.RunID != runID {
			continue
		}
		stage := stageIndex[attempt.Stage]
		if stage == nil {
			stage = &model.StageTotals{Stage: attempt.Stage}
			stageIndex[attempt.Stage] = stage
		}
		stage.Attempts++
		stage.DurationMS += attempt.Stats.DurationMS
		stage.ArtifactCount += len(attempt.Artifacts)
		stage.TotalCostUSD += attempt.Stats.Cost.TotalUSD
		stage.Substages = mergeSubstageTotals(stage.Substages, attempt.Stats.Substages)
		switch attempt.Status {
		case model.AttemptStatusSucceeded:
			stage.CompletedAttempts++
		case model.AttemptStatusBlocked:
			stage.BlockedAttempts++
		case model.AttemptStatusFailed:
			stage.FailedAttempts++
		}
	}

	stageNames := make([]string, 0, len(stageIndex))
	stats := model.RunStats{Currency: "USD"}
	for stageName, stage := range stageIndex {
		stage.TotalCostUSD = roundRunCurrency(stage.TotalCostUSD)
		stageNames = append(stageNames, stageName)
		stats.TotalCostUSD += stage.TotalCostUSD
		stats.TotalDurationMS += stage.DurationMS
		stats.CompletedAttempts += stage.CompletedAttempts
		stats.FailedAttempts += stage.FailedAttempts
		stats.BlockedAttempts += stage.BlockedAttempts
		stats.ArtifactCount += stage.ArtifactCount
	}
	sort.Strings(stageNames)
	stats.StageCount = len(stageNames)
	stats.Stages = make([]model.StageTotals, 0, len(stageNames))
	for _, stageName := range stageNames {
		stats.Stages = append(stats.Stages, *stageIndex[stageName])
	}
	stats.TotalCostUSD = roundRunCurrency(stats.TotalCostUSD)
	return stats
}

func mergeSubstageTotals(existing []model.SubstageTotal, timings []model.SubstageTiming) []model.SubstageTotal {
	index := make(map[string]int, len(existing))
	for i, item := range existing {
		index[item.Name] = i
	}
	for _, timing := range timings {
		if i, ok := index[timing.Name]; ok {
			existing[i].DurationMS += timing.DurationMS
			continue
		}
		index[timing.Name] = len(existing)
		existing = append(existing, model.SubstageTotal{
			Name:       timing.Name,
			DurationMS: timing.DurationMS,
		})
	}
	sort.Slice(existing, func(i, j int) bool {
		return existing[i].Name < existing[j].Name
	})
	return existing
}

func roundRunCurrency(value float64) float64 {
	return float64(int(value*1000000+0.5)) / 1000000
}

func (s *Service) emitSignals(event signals.PipelineEvent) error {
	synthesized, err := s.signals.RecordPipelineEvent(context.Background(), event)
	if err != nil {
		return err
	}
	for _, signal := range synthesized {
		if signal.IssueID == "" {
			continue
		}
		if err := s.commentIssue(signal.IssueID, fmt.Sprintf("Signal `%s` (%s): %s", signal.Category, signal.Severity, signal.Summary)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) commentIssue(issueID, body string) error {
	return s.store.Save(func(state *model.PersistedState) error {
		return s.recordComment(state, issueID, body)
	})
}

func serviceScope(projectPath string) string {
	projectPath = strings.TrimSuffix(projectPath, "/")
	if idx := strings.LastIndex(projectPath, "/"); idx >= 0 && idx+1 < len(projectPath) {
		return projectPath[idx+1:]
	}
	return projectPath
}

func severityForAttempt(status string, spec model.StageSpec) string {
	switch status {
	case model.AttemptStatusFailed:
		if severity := spec.SignalFailureSeverity(); severity != "" {
			return severity
		}
		return "medium"
	case model.AttemptStatusBlocked:
		return "medium"
	default:
		return "low"
	}
}

func normalizeLabels(existing, target []string) []string {
	labels := make([]string, 0, len(existing)+len(target))
	for _, label := range existing {
		if !strings.HasPrefix(label, "delivery/") {
			labels = append(labels, label)
		}
	}
	labels = append(labels, target...)
	slices.Sort(labels)
	return slices.Compact(labels)
}

func hasLabel(labels []string, want string) bool {
	return slices.Contains(labels, want)
}

func Pretty(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
