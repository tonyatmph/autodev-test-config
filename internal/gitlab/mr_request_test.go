package gitlab

import (
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestBuildImplementationMRRequest(t *testing.T) {
	proposal := MergeRequestProposal{
		IssueID:      "issue-1",
		RunID:        "run-1",
		DeliveryName: "example",
		Components: []ComponentMergeRequest{
			{Name: "api", BranchName: "autodev/run-1/api", TargetBranch: "main"},
		},
	}
	requests := BuildImplementationMRRequests(proposal)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	req := requests[0]
	if req.Request.SourceBranch != "autodev/run-1/api" || req.Request.TargetBranch != "main" {
		t.Fatalf("unexpected branches: %+v", req)
	}
	if len(req.Request.Description) == 0 || req.Request.Labels[0] != "autodev-implement" {
		t.Fatalf("unexpected request payload: %+v", req)
	}
}

func TestBuildPromotionMRRequest(t *testing.T) {
	proposal := PromotionProposal{
		DeliveryName: "example",
		Environment:  "dev",
		RunID:        "run-1",
		GitOpsRepo:   model.GitOpsTarget{ProjectPath: "gitops", Path: "clusters/dev"},
		Branch:       "main",
		FileChanges: []PromotionFile{
			{Path: "clusters/dev/app/app.yaml", Preview: "image: foo"},
		},
	}
	req := BuildPromotionMRRequest(proposal, "dev")
	if req.ProjectPath != "gitops" {
		t.Fatalf("unexpected project path: %+v", req)
	}
	if req.TargetBranch != "main" || req.SourceBranch != "main/run-1" {
		t.Fatalf("unexpected branches: %+v", req)
	}
	if len(req.Labels) != 2 || req.Labels[0] != "autodev-promotion" {
		t.Fatalf("unexpected labels: %+v", req)
	}
}
