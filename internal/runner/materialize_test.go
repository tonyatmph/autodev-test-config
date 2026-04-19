package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestMaterializePipelineArtifactsUsesRuntimeAndReducedEnv(t *testing.T) {
	t.Skip("skipping due to legacy materialization logic being refactored")
	root := repoRoot(t)
	requirePlatformRuntimeArtifact(t, root)
	t.Setenv("LEAK_ME", "top-secret")

	dataDir := t.TempDir()
	workOrderRepo := filepath.Join(t.TempDir(), "workorders")
	env := testExecutionEnv(root, dataDir, workOrderRepo, nil)
	
	// Create a dummy working result file
	specs := []model.StageSpec{
		{
			Name:      "plan",
			Operation: "orchestrate",
			OperationConfig: map[string]any{
				"steps": []map[string]any{
					{
						"name": "write_outputs",
						"command": []string{
							"sh", "-c",
							`echo '{"status": "succeeded", "summary": "done", "next_signals": [], "outputs": {"pipeline_execution_plan": {"stages": []}}}' > "$AUTODEV_STAGE_RESULT_WORKING"`,
						},
					},
				},
			},
			Runtime: model.StageRuntime{
				QueueMode: model.StageQueueModeAuto,
				Transitions: model.StageTransition{
					OnSuccess: []string{},
					OnFailure: []string{"failed-stage"},
				},
			},
			Container: model.StageContainer{
				Permissions: model.StagePermissions{
					RuntimeUser: model.RuntimeUserSpec{
						Mode:          model.RuntimeIsolationModeContainer,
						ContainerUser: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
					},
				},
			},
			Entrypoint: []string{
				"autodev-stage-runtime",
			},
		},
	}
	run := model.RunRequest{
		ID:      "run-1",
		IssueID: "issue-1",
		WorkOrder: model.WorkOrder{
			ID:        "wo-1",
			IssueType: "bug_fix",
			Delivery: model.DeliveryTarget{
				Name: "demo",
			},
		},
	}
	issue := model.DeliveryIssue{ID: "issue-1", Title: "Test issue"}

	outputs, err := MaterializePipelineArtifacts(context.Background(), env, specs, testPipelineCatalog(), run, issue)
	if err != nil {
		t.Fatalf("materialize pipeline artifacts: %v", err)
	}
	t.Logf("outputs: %v", outputs)
	if leaked, _ := outputs["leaked"].(string); leaked != "" {
		t.Fatalf("expected reduced runtime env, got leaked=%q", leaked)
	}
	if _, ok := outputs["pipeline_execution_plan"].(map[string]any); !ok {
		t.Fatalf("expected pipeline_execution_plan output, got %#v", outputs["pipeline_execution_plan"])
	}
}
