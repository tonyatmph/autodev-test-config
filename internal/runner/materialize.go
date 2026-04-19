package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/config"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

func MaterializePipelineArtifacts(ctx context.Context, env app.Env, specs []model.StageSpec, pipelineCatalog map[string]any, run model.RunRequest, issue model.DeliveryIssue) (map[string]any, error) {
	specMap := config.SpecMap(specs)
	planSpec, ok := specMap["plan"]
	if !ok {
		return nil, fmt.Errorf("plan stage spec is required for materialization")
	}
	operationPlan, err := planSpec.OrchestrationPlan()
	if err != nil {
		return nil, err
	}
	workspace := filepath.Join(env.DataDir, "workspaces", run.ID, planSpec.Name)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	contextPath := filepath.Join(workspace, "context.json")
	resultPath := filepath.Join(workspace, "result.json")
	stageNames := make([]string, 0, len(specs))
	for _, spec := range specs {
		stageNames = append(stageNames, spec.Name)
	}
	runtimeImages, err := stagecontainer.ResolveRuntimeImages(stageNames)
	if err != nil {
		return nil, err
	}
	contextPayload := map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage": map[string]any{
			"name":             planSpec.Name,
			"operation":        planSpec.Operation,
			"operation_plan":   operationPlan,
			"runtime":          planSpec.Runtime,
			"container":        planSpec.ContainerConfig(),
			"tooling_repo":     planSpec.ToolingRepo,
			"prompt_file":      planSpec.PromptFile,
			"success_criteria": planSpec.SuccessCriteriaContract(),
		},
		"run":        run,
		"attempt":    map[string]any{"id": "materialize", "stage": "plan", "attempt": 0},
		"issue":      issue,
		"work_order": run.CanonicalWorkOrder(),
		"invariants": map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    specs,
			"pipeline_catalog": pipelineCatalog,
			"runtime_images":   runtimeImagesForContract(runtimeImages),
		},
		"paths": map[string]any{
			"workspace":       workspace,
			"work_order_repo": env.WorkOrderRepo,
			"artifact_dir":    filepath.Join(env.DataDir, "artifacts"),
		},
	}
	if err := contracts.WriteFile(contextPath, contracts.StageContextSchema, contextPayload); err != nil {
		return nil, err
	}
	planImage, ok := runtimeImages[planSpec.Name]
	if !ok {
		return nil, fmt.Errorf("runtime image missing for stage %s", planSpec.Name)
	}
	
	dockerRunner := &stagecontainer.Docker{}
	if err := dockerRunner.Run(ctx, stagecontainer.Config{
		Env: env,
	}, planSpec, run.ID, planImage); err != nil {
		return nil, fmt.Errorf("materialize pipeline plan via stage runtime: %w", err)
	}
	
	var result struct {
		Outputs map[string]any `json:"outputs"`
	}
	if err := contracts.ReadFile(resultPath, contracts.StageResultSchema, &result); err != nil {
		return nil, err
	}
	return result.Outputs, nil
}

func runtimeImagesForContract(images map[string]stagecontainer.RuntimeImage) map[string]any {
	payload := make(map[string]any, len(images))
	for stage, image := range images {
		payload[stage] = map[string]any{
			"stage":  image.Stage,
			"image":  image.Ref,
			"digest": image.Digest,
		}
	}
	return payload
}
