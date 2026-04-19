package gitlab

import (
	"encoding/json"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestBuildMergeRequestProposalFromImplementationBundle(t *testing.T) {
	bundle := implementationBundle{
		DeliveryName: "example-delivery",
		DeployAsUnit: true,
		Components: []implementationComponent{
			{
				Name:             "api",
				Kind:             "api",
				ProjectPath:      "mph-tech/example-api",
				ResolvedRepoPath: "/tmp/example-api",
				BranchName:       "autodev/run/api",
				Source: model.SourceStamp{
					ProjectPath:   "mph-tech/example-api",
					DefaultBranch: "",
					CommitSHA:     "abc123",
					TreeState:     "clean",
				},
				RepoStatus:    "resolved",
				TopLevelFiles: []string{"main.go"},
				DependsOn:     []string{"core"},
				MutationPlan: &MutationPlan{
					BranchName: "autodev/run/api",
					BaseCommit: "abc123",
					TreeState:  "clean",
					RepoStatus: "resolved",
					Ready:      true,
					Steps: []PlanStep{{
						Name:        "prepare_branch",
						Description: "create branch",
						Commands:    []string{"git checkout -b autodev/run/api"},
					}},
				},
			},
		},
	}

	payload := map[string]any{"implementation_bundle": bundle}
	outputs, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	issue := model.DeliveryIssue{
		ID: "gitlab:1:1",
		WorkOrder: model.WorkOrder{
			Delivery: model.DeliveryTarget{
				Components: map[string]model.DeliveryComponent{
					"api": {
						Name: "api",
						Repo: model.RepoTarget{DefaultBranch: "main"},
					},
				},
			},
		},
	}
	run := model.RunRequest{ID: "run-1"}
	stage := model.StageResult{Outputs: outputs}

	proposal, err := BuildMergeRequestProposal(issue, run, stage)
	if err != nil {
		t.Fatalf("build proposal: %v", err)
	}
	if proposal.IssueID != issue.ID {
		t.Fatalf("expected issue id %s, got %s", issue.ID, proposal.IssueID)
	}
	if len(proposal.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(proposal.Components))
	}
	comp := proposal.Components[0]
	if comp.TargetBranch != "main" {
		t.Fatalf("expected target branch derived from work order, got %s", comp.TargetBranch)
	}
	if comp.BranchName != "autodev/run/api" {
		t.Fatalf("unexpected branch name: %s", comp.BranchName)
	}
	if comp.MutationPlan == nil || !comp.MutationPlan.Ready {
		t.Fatalf("mutation plan not ready or missing")
	}
}

func TestBuildMergeRequestProposalMissingBundle(t *testing.T) {
	_, err := BuildMergeRequestProposal(
		model.DeliveryIssue{ID: "gitlab:1:1"},
		model.RunRequest{ID: "run-1"},
		model.StageResult{Outputs: json.RawMessage(`{"component_source_stamps":[]}`)},
	)
	if err == nil {
		t.Fatalf("expected error for missing bundle")
	}
}
