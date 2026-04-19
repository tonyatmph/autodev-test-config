package policy

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestEvaluateStageBlocksWhenDocumentationSurfaceMissing(t *testing.T) {
	decision := EvaluateStage(
		model.StageSpec{
			Name:            "review",
			Operation:       "orchestrate",
			OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
			Runtime:         model.StageRuntime{ReportStages: []string{"test", "security"}},
		},
		model.RunRequest{
			WorkOrder: model.WorkOrder{
				Delivery: model.DeliveryTarget{
					SelectedComponents: []string{"api"},
					Documentation: model.DocumentationPolicy{
						Required:      true,
						DocsComponent: "docs",
					},
					Components: model.DeliveryComponents{
						"api":  {Name: "api"},
						"docs": {Name: "docs"},
					},
				},
			},
		},
		model.DeliveryIssue{},
	)

	if !decision.Blocked {
		t.Fatal("expected documentation policy to block review")
	}
}

func TestEvaluateStageBlocksPromoteProdWithoutApproval(t *testing.T) {
	decision := EvaluateStage(
		model.StageSpec{Name: "promote_prod", Operation: "orchestrate", OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}}, ApprovalRequired: true},
		model.RunRequest{},
		model.DeliveryIssue{Approval: model.ApprovalGate{Label: "delivery/approved"}},
	)

	if !decision.Blocked {
		t.Fatal("expected prod promotion to block without approval")
	}
}

func TestEvaluateStageBlocksWhenIdentityRulesMissing(t *testing.T) {
	decision := EvaluateStage(
		model.StageSpec{
			Name:            "implement",
			Operation:       "orchestrate",
			OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
			Container: model.StageContainer{
				RunAs:   model.ExecutionIdentityGenerator,
				WriteAs: model.ExecutionIdentityGenerator,
				Permissions: model.StagePermissions{
					Writable: []string{string(model.StageSurfaceComponents)},
				},
			},
		},
		model.RunRequest{
			WorkOrder: model.WorkOrder{
				Delivery: model.DeliveryTarget{
					SelectedComponents: []string{"api"},
					Components: model.DeliveryComponents{
						"api": {Name: "api"},
					},
				},
			},
		},
		model.DeliveryIssue{},
	)

	if !decision.Blocked {
		t.Fatal("expected implement stage to block when generator identity has no paths")
	}
}

func TestEvaluateStageAllowsGovernedReleasePrepareWithJournalTarget(t *testing.T) {
	decision := EvaluateStage(
		model.StageSpec{
			Name:            "release_prepare",
			Operation:       "orchestrate",
			OperationConfig: map[string]any{"steps": []map[string]any{{"name": "step", "command": []string{"echo", "ok"}}}},
			Runtime:         model.StageRuntime{ReportStages: []string{"test", "security", "review"}},
			Container: model.StageContainer{
				RunAs:   model.ExecutionIdentityGoverned,
				WriteAs: model.ExecutionIdentityGoverned,
				Permissions: model.StagePermissions{
					Writable: []string{string(model.StageSurfaceJournal)},
				},
			},
		},
		model.RunRequest{
			WorkOrder: model.WorkOrder{
				Delivery: model.DeliveryTarget{
					Journal: model.JournalTarget{
						Repo:     model.RepoTarget{ProjectPath: "work-orders"},
						Path:     "runs",
						Strategy: "git",
					},
				},
			},
		},
		model.DeliveryIssue{},
	)

	if decision.Blocked {
		t.Fatalf("expected governed release_prepare to pass with journal target, got %+v", decision)
	}
}
