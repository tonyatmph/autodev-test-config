package pipeline

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestPlanRunnableStagesInjectsDependencies(t *testing.T) {
	plan := &model.PipelineExecutionPlan{
		Stages: []model.PipelineExecutionStage{
			{Name: "alpha"},
			{Name: "beta", Dependencies: []string{"alpha"}},
		},
	}
	byStage := map[string]model.StageAttempt{}
	runnable := PlanRunnableStages(plan, byStage, true)
	if len(runnable) != 1 || runnable[0].Name != "alpha" {
		t.Fatalf("expected alpha to be runnable, got %v", runnable)
	}
	byStage["alpha"] = model.StageAttempt{Stage: "alpha", Status: model.AttemptStatusSucceeded}
	runnable = PlanRunnableStages(plan, byStage, true)
	if len(runnable) != 1 || runnable[0].Name != "beta" {
		t.Fatalf("expected beta to be runnable after alpha, got %v", runnable)
	}
}

func TestRunStatusFromPlanHonorsCheckpoints(t *testing.T) {
	plan := &model.PipelineExecutionPlan{
		Stages: []model.PipelineExecutionStage{
			{Name: "complete", Checkpoint: model.RunStatusCompleted},
		},
	}
	byStage := map[string]model.StageAttempt{
		"complete": {Stage: "complete", Status: model.AttemptStatusSucceeded},
	}
	status := RunStatusFromPlan(plan, byStage, model.DeliveryTarget{}, false)
	if status != model.RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", status)
	}
}
