package pipeline

import "g7.mph.tech/mph-tech/autodev/internal/model"

type Evaluation struct {
	Triggered []model.StageSpec
	Runnable  []model.StageSpec
	Status    string
}

func Evaluate(plan *model.PipelineExecutionPlan, run model.RunRequest, byStage map[string]model.StageAttempt, approved bool) Evaluation {
	specs := PlanSpecs(plan)
	specMap := PlanSpecsMap(plan)
	ordered := SpecsInOrder(specs)

	triggered := make([]model.StageSpec, 0)
	runnable := make([]model.StageSpec, 0)
	hasActive := false
	hasFailed := false
	hasEligibleRollback := false

	for _, spec := range ordered {
		attempt, exists := byStage[spec.Name]

		if !exists {
			if !spec.AutoQueue() && triggeredByFailure(spec, run, byStage) || !spec.AutoQueue() && triggeredBySuccess(spec, byStage) {
				triggered = append(triggered, spec)
			}
			if isReady(spec, byStage, approved) {
				runnable = append(runnable, spec)
			}
			continue
		}

		if attempt.Status == model.AttemptStatusSucceeded {
			if spec.CompletionCheckpoint() {
				return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusCompleted}
			}
			if spec.FailureCheckpoint() {
				return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusFailed}
			}
			if spec.ApprovalCheckpoint() && !approved {
				return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusAwaitingApproval}
			}
			hasActive = true
			continue
		}

		if attempt.Status == model.AttemptStatusRunning {
			hasActive = true
			continue
		}

		if attempt.Status == model.AttemptStatusFailed {
			hasFailed = true
			if rollbackEligible(spec, run.DeliveryTarget(), byStage) {
				hasEligibleRollback = true
			}
		}
	}

	if hasFailed && !hasEligibleRollback {
		return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusFailed}
	}
	if hasActive || hasEligibleRollback || len(runnable) > 0 || len(triggered) > 0 {
		return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusActive}
	}
	for _, spec := range specMap {
		if _, ok := byStage[spec.Name]; ok {
			return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusActive}
		}
	}
	return Evaluation{Triggered: triggered, Runnable: runnable, Status: model.RunStatusPending}
}

func isReady(spec model.StageSpec, byStage map[string]model.StageAttempt, approved bool) bool {
	if !spec.AutoQueue() {
		return false
	}
	if spec.ApprovalRequired && !approved {
		return false
	}
	for _, dep := range spec.Dependencies {
		attempt, ok := byStage[dep]
		if !ok || attempt.Status != model.AttemptStatusSucceeded {
			return false
		}
	}
	_, exists := byStage[spec.Name]
	return !exists
}

func triggeredByFailure(spec model.StageSpec, run model.RunRequest, byStage map[string]model.StageAttempt) bool {
	for _, stageName := range spec.Runtime.TriggerOnFailure {
		attempt, ok := byStage[stageName]
		if !ok || attempt.Status != model.AttemptStatusFailed {
			continue
		}
		if spec.Runtime.Rollback.Policy == "" {
			return true
		}
		if rollbackPolicyEnabled(run.DeliveryTarget(), spec.Environment(), spec.Runtime.Rollback.Policy) {
			return true
		}
	}
	return false
}

func triggeredBySuccess(spec model.StageSpec, byStage map[string]model.StageAttempt) bool {
	for _, stageName := range spec.Runtime.TriggerOnSuccess {
		attempt, ok := byStage[stageName]
		if ok && attempt.Status == model.AttemptStatusSucceeded {
			return true
		}
	}
	return false
}

