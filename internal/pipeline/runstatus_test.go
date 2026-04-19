package pipeline

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestRunStatusStaysActiveWhenRollbackIsEligible(t *testing.T) {
	specs := map[string]model.StageSpec{
		"observe_dev": {
			Name: "observe_dev",
			Runtime: model.StageRuntime{
				Environment: "dev",
				Rollback: model.StageRollbackPlan{
					Stage:  "rollback_dev",
					Policy: "app_auto_rollback",
				},
			},
		},
	}
	byStage := map[string]model.StageAttempt{
		"observe_dev": {
			Stage:  "observe_dev",
			Status: model.AttemptStatusFailed,
		},
	}
	target := model.DeliveryTarget{
		Environments: model.PromotionTargets{
			Dev: model.EnvironmentTarget{
				RollbackPolicy: model.RollbackPolicy{
					AppAutoRollback: true,
				},
			},
		},
	}

	if got := Evaluate(testPlan([]model.StageSpec{specs["observe_dev"]}), model.RunRequest{WorkOrder: model.WorkOrder{Delivery: target}}, byStage, false).Status; got != model.RunStatusActive {
		t.Fatalf("expected active status while rollback is eligible, got %s", got)
	}
}

func TestRunStatusFailsAfterRollbackObservation(t *testing.T) {
	specs := map[string]model.StageSpec{
		"observe_rollback_dev": {
			Name:    "observe_rollback_dev",
			Runtime: model.StageRuntime{CheckpointStatus: model.RunStatusFailed},
		},
	}
	byStage := map[string]model.StageAttempt{
		"observe_dev": {
			Stage:  "observe_dev",
			Status: model.AttemptStatusFailed,
		},
		"observe_rollback_dev": {
			Stage:  "observe_rollback_dev",
			Status: model.AttemptStatusSucceeded,
		},
	}

	if got := Evaluate(testPlan([]model.StageSpec{specs["observe_rollback_dev"]}), model.RunRequest{}, byStage, false).Status; got != model.RunStatusFailed {
		t.Fatalf("expected failed status after rollback observation, got %s", got)
	}
}

func TestRunStatusFailsWhenMixedWithEarlierSuccesses(t *testing.T) {
	specs := map[string]model.StageSpec{
		"intake": {
			Name: "intake",
		},
		"plan": {
			Name: "plan",
		},
		"implement": {
			Name: "implement",
		},
	}
	byStage := map[string]model.StageAttempt{
		"intake": {
			Stage:  "intake",
			Status: model.AttemptStatusSucceeded,
		},
		"plan": {
			Stage:  "plan",
			Status: model.AttemptStatusSucceeded,
		},
		"implement": {
			Stage:  "implement",
			Status: model.AttemptStatusFailed,
		},
	}

	if got := Evaluate(testPlan([]model.StageSpec{specs["intake"], specs["plan"], specs["implement"]}), model.RunRequest{}, byStage, false).Status; got != model.RunStatusFailed {
		t.Fatalf("expected failed status when a later stage fails, got %s", got)
	}
}
