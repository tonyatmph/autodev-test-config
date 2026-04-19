package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

// MergeRequestProposal is a first-pass payload to describe a GitLab MR
// that implements the bundle produced by the implement stage.
type MergeRequestProposal struct {
	IssueID      string                  `json:"issue_id"`
	RunID        string                  `json:"run_id"`
	DeliveryName string                  `json:"delivery_name"`
	DeployAsUnit bool                    `json:"deploy_as_unit"`
	Components   []ComponentMergeRequest `json:"components"`
}

// ComponentMergeRequest captures the plan for a single component.
type ComponentMergeRequest struct {
	Name             string            `json:"name"`
	Kind             string            `json:"kind"`
	ProjectPath      string            `json:"project_path"`
	ResolvedRepoPath string            `json:"resolved_repo_path,omitempty"`
	BranchName       string            `json:"branch_name"`
	TargetBranch     string            `json:"target_branch"`
	RepoStatus       string            `json:"repo_status"`
	Source           model.SourceStamp `json:"source"`
	CandidateFiles   []string          `json:"candidate_files,omitempty"`
	DependsOn        []string          `json:"depends_on,omitempty"`
	MutationPlan     *MutationPlan     `json:"mutation_plan,omitempty"`
}

// MutationPlan mirrors the JSON payload produced by the implement stage.
type MutationPlan struct {
	BranchName     string     `json:"branch_name"`
	BaseCommit     string     `json:"base_commit"`
	TreeState      string     `json:"tree_state"`
	RepoStatus     string     `json:"repo_status"`
	Ready          bool       `json:"ready"`
	BranchExists   bool       `json:"branch_exists"`
	CandidateFiles []string   `json:"candidate_files,omitempty"`
	Issues         []string   `json:"issues,omitempty"`
	Steps          []PlanStep `json:"steps"`
}

// PlanStep is a step inside a mutation plan.
type PlanStep struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Commands       []string `json:"commands,omitempty"`
	CandidatePaths []string `json:"candidate_paths,omitempty"`
}

type implementationComponent struct {
	Name             string            `json:"name"`
	Kind             string            `json:"kind"`
	ProjectPath      string            `json:"project_path"`
	ResolvedRepoPath string            `json:"resolved_repo_path,omitempty"`
	BranchName       string            `json:"branch_name"`
	Source           model.SourceStamp `json:"source"`
	RepoStatus       string            `json:"repo_status"`
	TopLevelFiles    []string          `json:"top_level_files,omitempty"`
	DependsOn        []string          `json:"depends_on,omitempty"`
	MutationPlan     *MutationPlan     `json:"mutation_plan,omitempty"`
}

type implementationBundle struct {
	DeliveryName        string                    `json:"delivery_name"`
	DeployAsUnit        bool                      `json:"deploy_as_unit"`
	ComponentCount      int                       `json:"component_count"`
	ResolvedRepoCount   int                       `json:"resolved_repo_count"`
	UnresolvedRepoCount int                       `json:"unresolved_repo_count"`
	Components          []implementationComponent `json:"components"`
}

var ErrMissingImplementationBundle = errors.New("stage outputs missing implementation bundle")

// BuildMergeRequestProposal turns the implement stage output into a proposal
// that can be used to populate a GitLab merge request payload later.
func BuildMergeRequestProposal(issue model.DeliveryIssue, run model.RunRequest, stage model.StageResult) (MergeRequestProposal, error) {
	bundle, err := decodeImplementationBundle(stage.Outputs)
	if err != nil {
		return MergeRequestProposal{}, err
	}
	if len(bundle.Components) == 0 {
		return MergeRequestProposal{}, fmt.Errorf("implementation bundle for issue %s contains no components", issue.ID)
	}

	proposal := MergeRequestProposal{
		IssueID:      issue.ID,
		RunID:        run.ID,
		DeliveryName: bundle.DeliveryName,
		DeployAsUnit: bundle.DeployAsUnit,
		Components:   make([]ComponentMergeRequest, 0, len(bundle.Components)),
	}

	for _, component := range bundle.Components {
		targetBranch := component.Source.DefaultBranch
		if targetBranch == "" {
			if cfg, ok := issue.WorkOrder.Delivery.Components[component.Name]; ok {
				targetBranch = cfg.Repo.DefaultBranch
			}
		}
		proposal.Components = append(proposal.Components, ComponentMergeRequest{
			Name:             component.Name,
			Kind:             component.Kind,
			ProjectPath:      component.ProjectPath,
			ResolvedRepoPath: component.ResolvedRepoPath,
			BranchName:       component.BranchName,
			TargetBranch:     targetBranch,
			RepoStatus:       component.RepoStatus,
			Source:           component.Source,
			CandidateFiles:   append([]string(nil), component.TopLevelFiles...),
			DependsOn:        append([]string(nil), component.DependsOn...),
			MutationPlan:     component.MutationPlan,
		})
	}

	return proposal, nil
}

func decodeImplementationBundle(outputs json.RawMessage) (implementationBundle, error) {
	if len(outputs) == 0 {
		return implementationBundle{}, ErrMissingImplementationBundle
	}
	var payload struct {
		Bundle implementationBundle `json:"implementation_bundle"`
	}
	if err := json.Unmarshal(outputs, &payload); err != nil {
		return implementationBundle{}, fmt.Errorf("decode implementation bundle: %w", err)
	}
	if payload.Bundle.DeliveryName == "" && len(payload.Bundle.Components) == 0 {
		return implementationBundle{}, ErrMissingImplementationBundle
	}
	return payload.Bundle, nil
}
