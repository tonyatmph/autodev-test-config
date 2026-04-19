package workorder

import (
	"fmt"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

var pipelinePlanKeys = []string{
	"pipeline_intent",
	"policy_evaluation",
	"pipeline_build_plan",
	"pipeline_execution_plan",
}

// WritePipelineArtifacts projects first-class meta-pipeline artifacts into dedicated
// files under the work-order run journal.
func WritePipelineArtifacts(repoPath string, run model.RunRequest, outputs map[string]any) ([]string, error) {
	if repoPath == "" || len(outputs) == 0 {
		return nil, nil
	}
	base := filepath.Join(repoPath, "work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID), "pipeline")
	written := make([]string, 0, len(pipelinePlanKeys))
	for _, key := range pipelinePlanKeys {
		value, ok := outputs[key]
		if !ok {
			continue
		}
		path := filepath.Join(base, key+".json")
		if err := contracts.WriteFile(path, schemaForPipelineArtifact(key), value); err != nil {
			return nil, err
		}
		written = append(written, filepath.ToSlash(filepath.Join("work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID), "pipeline", key+".json")))
	}
	return written, nil
}

// PipelineArtifactPaths returns the expected journal paths for persisted plan artifacts.
func PipelineArtifactPaths(run model.RunRequest) map[string]string {
	base := filepath.ToSlash(filepath.Join("work-orders", canonicalWorkOrderID(run), "runs", sanitizePathSegment(run.ID), "pipeline"))
	return map[string]string{
		"pipeline_intent":         base + "/pipeline_intent.json",
		"policy_evaluation":       base + "/policy_evaluation.json",
		"pipeline_build_plan":     base + "/pipeline_build_plan.json",
		"pipeline_execution_plan": base + "/pipeline_execution_plan.json",
	}
}

func schemaForPipelineArtifact(name string) string {
	switch name {
	case "pipeline_intent":
		return contracts.PipelineIntentSchema
	case "policy_evaluation":
		return contracts.PolicyEvaluationSchema
	case "pipeline_build_plan":
		return contracts.PipelineBuildPlanSchema
	case "pipeline_execution_plan":
		return contracts.PipelineExecutionPlanSchema
	default:
		panic(fmt.Sprintf("unknown pipeline artifact schema for %s", name))
	}
}
