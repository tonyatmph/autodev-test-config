package gitlab

import (
	"encoding/json"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestBuildPromotionProposal(t *testing.T) {
	issue := model.DeliveryIssue{ID: "issue-1"}
	run := model.RunRequest{ID: "run-1"}
	plan := map[string]any{
		"promotion_plan": map[string]any{
			"stage":               "promote_dev",
			"environment":         "dev",
			"gitops_repo":         map[string]any{"project_path": "example/gitops", "path": "clusters/dev/app", "promotion_branch": "main", "environment": "dev"},
			"branch":              "autodev/run-1/dev",
			"application_digest":  "sha256:deadbeef",
			"infrastructure_ref":  "infra@v1",
			"database_bundle_ref": "db-v2",
			"steps":               []string{"step1", "step2"},
			"files": []map[string]any{
				{"file": "clusters/dev/app/app.yaml", "operation": "update", "description": "app", "preview": "image: sha"},
			},
			"ready":         true,
			"issues":        []string{},
			"summary":       "promotion ready",
			"gitops_commit": "abcdef",
		},
	}
	out, _ := json.Marshal(plan)
	result := model.StageResult{Outputs: out}
	proposal, err := BuildPromotionProposal(issue, run, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal.Environment != "dev" {
		t.Fatalf("expected dev environment, got %q", proposal.Environment)
	}
	if proposal.GitOpsCommit != "abcdef" {
		t.Fatalf("expected commit, got %q", proposal.GitOpsCommit)
	}
	if len(proposal.FileChanges) != 1 || proposal.FileChanges[0].Path != "clusters/dev/app/app.yaml" {
		t.Fatalf("unexpected file change: %+v", proposal.FileChanges)
	}
}
