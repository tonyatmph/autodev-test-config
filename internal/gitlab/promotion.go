package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

var errMissingPromotionPlan = errors.New("stage outputs missing promotion plan")

// PromotionProposal describes how to update a GitOps repo for a promotion stage.
type PromotionProposal struct {
	IssueID           string             `json:"issue_id"`
	RunID             string             `json:"run_id"`
	Stage             string             `json:"stage"`
	Environment       string             `json:"environment"`
	DeliveryName      string             `json:"delivery_name"`
	GitOpsRepo        model.GitOpsTarget `json:"gitops_repo"`
	Branch            string             `json:"branch"`
	ApplicationDigest string             `json:"application_digest"`
	InfrastructureRef string             `json:"infrastructure_ref"`
	DatabaseBundleRef string             `json:"database_bundle_ref"`
	GitOpsCommit      string             `json:"gitops_commit,omitempty"`
	Ready             bool               `json:"ready"`
	Issues            []string           `json:"issues,omitempty"`
	Summary           string             `json:"summary"`
	Steps             []string           `json:"steps,omitempty"`
	FileChanges       []PromotionFile    `json:"file_changes,omitempty"`
}

// PromotionFile is a single file change within the promotion plan.
type PromotionFile struct {
	Path        string `json:"path"`
	Operation   string `json:"operation"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// BuildPromotionProposal turns a promotion stage output into a typed GitLab proposal.
func BuildPromotionProposal(issue model.DeliveryIssue, run model.RunRequest, stage model.StageResult) (PromotionProposal, error) {
	plan, err := decodePromotionPlan(stage.Outputs)
	if err != nil {
		return PromotionProposal{}, err
	}
	proposal := PromotionProposal{
		IssueID:           issue.ID,
		RunID:             run.ID,
		Stage:             plan.Stage,
		Environment:       plan.Environment,
		DeliveryName:      plan.Summary,
		GitOpsRepo:        plan.GitOpsRepo,
		Branch:            plan.Branch,
		ApplicationDigest: plan.ApplicationDigest,
		InfrastructureRef: plan.InfrastructureRef,
		DatabaseBundleRef: plan.DatabaseBundleRef,
		GitOpsCommit:      plan.GitOpsCommit,
		Ready:             plan.Ready,
		Issues:            append([]string(nil), plan.Issues...),
		Summary:           plan.Summary,
		Steps:             append([]string(nil), plan.Steps...),
		FileChanges:       make([]PromotionFile, 0, len(plan.Files)),
	}
	for _, file := range plan.Files {
		proposal.FileChanges = append(proposal.FileChanges, PromotionFile{
			Path:        file.Path,
			Operation:   file.Operation,
			Description: file.Description,
			Preview:     file.Preview,
		})
	}
	return proposal, nil
}

type promotionPlanOutput struct {
	Plan promotionPlanPayload `json:"promotion_plan"`
}

type promotionPlanPayload struct {
	Stage             string                 `json:"stage"`
	Environment       string                 `json:"environment"`
	GitOpsRepo        model.GitOpsTarget     `json:"gitops_repo"`
	Branch            string                 `json:"branch"`
	ApplicationDigest string                 `json:"application_digest"`
	InfrastructureRef string                 `json:"infrastructure_ref"`
	DatabaseBundleRef string                 `json:"database_bundle_ref"`
	Steps             []string               `json:"steps"`
	Files             []promotionFilePayload `json:"files"`
	Ready             bool                   `json:"ready"`
	Issues            []string               `json:"issues"`
	Summary           string                 `json:"summary"`
	GitOpsCommit      string                 `json:"gitops_commit"`
}

type promotionFilePayload struct {
	Path        string `json:"file"`
	Operation   string `json:"operation"`
	Description string `json:"description"`
	Preview     string `json:"preview"`
}

func decodePromotionPlan(outputs json.RawMessage) (promotionPlanPayload, error) {
	if len(outputs) == 0 {
		return promotionPlanPayload{}, errMissingPromotionPlan
	}
	var payload promotionPlanOutput
	if err := json.Unmarshal(outputs, &payload); err != nil {
		return promotionPlanPayload{}, fmt.Errorf("decode promotion plan: %w", err)
	}
	if payload.Plan.Stage == "" && len(payload.Plan.Files) == 0 {
		return promotionPlanPayload{}, errMissingPromotionPlan
	}
	return payload.Plan, nil
}
