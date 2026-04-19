package pipeline

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func testPlan(specs []model.StageSpec) *model.PipelineExecutionPlan {
	stages := make([]model.PipelineExecutionStage, 0, len(specs))
	for _, spec := range specs {
		stages = append(stages, model.PipelineExecutionStage{
			Name:         spec.Name,
			Spec:         spec,
			Dependencies: append([]string(nil), spec.Dependencies...),
			QueueMode:    spec.QueueMode(),
			Checkpoint:   spec.Runtime.CheckpointStatus,
		})
	}
	return &model.PipelineExecutionPlan{Stages: stages}
}

func TestRunnableStagesFansOutFromImplement(t *testing.T) {
	specs := []model.StageSpec{
		{Name: "test", Dependencies: []string{"implement"}, Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
		{Name: "security", Dependencies: []string{"implement"}, Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
		{Name: "review", Dependencies: []string{"plan", "implement"}, Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
	}
	byStage := map[string]model.StageAttempt{
		"plan":      {Stage: "plan", Status: model.AttemptStatusSucceeded},
		"implement": {Stage: "implement", Status: model.AttemptStatusSucceeded},
	}

	runnable := Evaluate(testPlan(specs), model.RunRequest{}, byStage, false).Runnable
	if len(runnable) != 3 {
		t.Fatalf("expected 3 runnable stages, got %d", len(runnable))
	}
}

func TestPromoteProdRequiresApproval(t *testing.T) {
	spec := model.StageSpec{
		Name:             "promote_prod",
		Dependencies:     []string{"observe_dev"},
		ApprovalRequired: true,
	}
	byStage := map[string]model.StageAttempt{
		"observe_dev": {Stage: "observe_dev", Status: model.AttemptStatusSucceeded},
	}

	if ready := Evaluate(testPlan([]model.StageSpec{spec}), model.RunRequest{}, byStage, false).Runnable; len(ready) != 0 {
		t.Fatal("promote_prod should not be ready without approval")
	}
	if ready := Evaluate(testPlan([]model.StageSpec{spec}), model.RunRequest{}, byStage, true).Runnable; len(ready) != 1 || ready[0].Name != "promote_prod" {
		t.Fatal("promote_prod should be ready once approval is present")
	}
}

func TestRollbackStagesAreNotGenericRunnable(t *testing.T) {
	spec := model.StageSpec{
		Name:         "rollback_dev",
		Dependencies: []string{"observe_dev"},
		Runtime:      model.StageRuntime{QueueMode: model.StageQueueModeTriggered},
	}
	byStage := map[string]model.StageAttempt{
		"observe_dev": {Stage: "observe_dev", Status: model.AttemptStatusSucceeded},
	}

	if ready := Evaluate(testPlan([]model.StageSpec{spec}), model.RunRequest{}, byStage, true).Runnable; len(ready) != 0 {
		t.Fatal("rollback_dev should only be queued by rollback orchestration, not generic DAG scheduling")
	}
}

func TestGenerateBlocksDependentStages(t *testing.T) {
	specs := []model.StageSpec{
		{Name: "implement", Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
		{Name: "generate", Dependencies: []string{"implement"}, Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
		{Name: "test", Dependencies: []string{"generate"}, Runtime: model.StageRuntime{QueueMode: model.StageQueueModeAuto}},
	}
	byStage := map[string]model.StageAttempt{
		"implement": {Stage: "implement", Status: model.AttemptStatusSucceeded},
	}

	runnable := Evaluate(testPlan(specs), model.RunRequest{}, byStage, false).Runnable
	if len(runnable) != 1 || runnable[0].Name != "generate" {
		t.Fatalf("expected only generate to be runnable, got %v", runnable)
	}
}
