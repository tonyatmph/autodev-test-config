package pipeline

import (
	"fmt"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

// PlanSpecs converts a materialized PipelineExecutionPlan into StageSpecs that can drive
// queueing and run-status decisions. nil plans produce an empty slice.
func PlanSpecs(plan *model.PipelineExecutionPlan) []model.StageSpec {
	if plan == nil {
		return nil
	}
	specs := make([]model.StageSpec, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		specs = append(specs, stageToSpec(stage))
	}
	return specs
}

// PlanSpecsMap returns the same specs keyed by stage name.
func PlanSpecsMap(plan *model.PipelineExecutionPlan) map[string]model.StageSpec {
	specs := PlanSpecs(plan)
	m := make(map[string]model.StageSpec, len(specs))
	for _, spec := range specs {
		m[spec.Name] = spec
	}
	return m
}

// PlanRunnableStages uses the materialized plan to determine which stages can be queued.
func PlanRunnableStages(plan *model.PipelineExecutionPlan, byStage map[string]model.StageAttempt, approved bool) []model.StageSpec {
	return Evaluate(plan, model.RunRequest{}, byStage, approved).Runnable
}

// RunStatusFromPlan evaluates the run status from the materialized plan rather than the static catalog.
func RunStatusFromPlan(plan *model.PipelineExecutionPlan, byStage map[string]model.StageAttempt, target model.DeliveryTarget, approved bool) string {
	run := model.RunRequest{WorkOrder: model.WorkOrder{Delivery: target}}
	return Evaluate(plan, run, byStage, approved).Status
}

func BuildExecutionPlan(run model.RunRequest, specs []model.StageSpec, runtimeImages map[string]stagecontainer.RuntimeImage) (model.PipelineExecutionPlan, error) {
	if len(specs) == 0 {
		return model.PipelineExecutionPlan{}, fmt.Errorf("pipeline execution plan requires at least one stage spec")
	}
	ordered := SpecsInOrder(specs)
	stages := make([]model.PipelineExecutionStage, 0, len(ordered))
	checkpoint := ""
	for _, spec := range ordered {
		image, ok := runtimeImages[spec.Name]
		if !ok {
			return model.PipelineExecutionPlan{}, fmt.Errorf("pipeline execution plan missing runtime image for stage %s", spec.Name)
		}
		if checkpoint == "" && strings.TrimSpace(spec.Runtime.CheckpointStatus) != "" {
			checkpoint = strings.TrimSpace(spec.Runtime.CheckpointStatus)
		}
		stages = append(stages, model.PipelineExecutionStage{
			Name:            spec.Name,
			Spec:            spec,
			Dependencies:    cloneStrings(spec.Dependencies),
			Environment:     spec.Environment(),
			QueueMode:       spec.QueueMode(),
			Transitions:     spec.Runtime.Transitions,
			Checkpoint:      spec.Runtime.CheckpointStatus,
			RunAs:           spec.RunAsIdentity(),
			WriteAs:         spec.WriteAsIdentity(),
			Container:       spec.ContainerConfig(),
			SuccessCriteria: spec.SuccessCriteriaContract(),
			OutputArtifacts: cloneOutputArtifacts(spec.Runtime.OutputArtifacts),
			ReportStages:    cloneStrings(spec.Runtime.ReportStages),
			Image:           image.Ref,
			ImageDigest:     image.Digest,
			Entrypoint:      cloneStrings(spec.Entrypoint),
		})
	}
	order := run.CanonicalWorkOrder()
	return model.PipelineExecutionPlan{
		SchemaVersion:          "autodev-pipeline-execution-plan-v1",
		RunID:                  run.ID,
		IssueID:                run.IssueID,
		WorkOrderID:            order.ID,
		IssueType:              order.IssueType,
		PipelineFamily:         order.PipelineTemplate,
		PipelineSelection:      "selected",
		Testing:                order.Testing,
		DeliveryName:           order.Delivery.Name,
		Checkpoint:             checkpoint,
		Stages:                 stages,
	}, nil
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}

func cloneOutputArtifacts(values []model.StageOutputArtifact) []model.StageOutputArtifact {
	if len(values) == 0 {
		return []model.StageOutputArtifact{}
	}
	return append([]model.StageOutputArtifact{}, values...)
}

func stageToSpec(stage model.PipelineExecutionStage) model.StageSpec {
	if stage.Spec.Name != "" {
		return stage.Spec
	}
	spec := model.StageSpec{
		Name:         stage.Name,
		Dependencies: append([]string(nil), stage.Dependencies...),
		Runtime: model.StageRuntime{
			QueueMode:        stage.QueueMode,
			Environment:      stage.Environment,
			CheckpointStatus: stage.Checkpoint,
			SuccessCriteria:  stage.SuccessCriteria,
			ReportStages:     append([]string(nil), stage.ReportStages...),
			Transitions:      stage.Transitions,
		},
		Container: stage.Container,
	}
	if len(stage.OutputArtifacts) > 0 {
		spec.Runtime.OutputArtifacts = append([]model.StageOutputArtifact(nil), stage.OutputArtifacts...)
	}
	return spec
}
