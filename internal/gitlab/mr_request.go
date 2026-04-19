package gitlab

import (
	"fmt"
	"strings"
)

// MergeRequestCreatePayload mirrors the GitLab MR create API.
type MergeRequestCreatePayload struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels,omitempty"`
	RemoveSource bool     `json:"remove_source_branch"`
}

// PromotionMergeRequestPayload captures the GitLab MR intent for promotion stages.
type PromotionMergeRequestPayload struct {
	ProjectPath  string   `json:"project_path"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels,omitempty"`
}

type ImplementationMergeRequestPayload struct {
	ProjectPath string                    `json:"project_path"`
	Component   ComponentMergeRequest     `json:"component"`
	Request     MergeRequestCreatePayload `json:"request"`
}

func BuildImplementationMRRequests(proposal MergeRequestProposal) []ImplementationMergeRequestPayload {
	requests := make([]ImplementationMergeRequestPayload, 0, len(proposal.Components))
	for _, component := range proposal.Components {
		title := fmt.Sprintf("[autodev] Implement %s (%s)", proposal.DeliveryName, component.Name)
		desc := fmt.Sprintf("Implements component %s (%s).\n\nIssue: %s\nRun: %s", component.Name, component.Kind, proposal.IssueID, proposal.RunID)
		requests = append(requests, ImplementationMergeRequestPayload{
			ProjectPath: component.ProjectPath,
			Component:   component,
			Request: MergeRequestCreatePayload{
				Title:        title,
				Description:  desc,
				SourceBranch: component.BranchName,
				TargetBranch: component.TargetBranch,
				Labels:       []string{"autodev-implement", fmt.Sprintf("component:%s", component.Name)},
				RemoveSource: true,
			},
		})
	}
	return requests
}

func BuildPromotionMRRequest(proposal PromotionProposal, environment string) PromotionMergeRequestPayload {
	title := fmt.Sprintf("[autodev] Promote %s to %s", proposal.DeliveryName, environment)
	desc := fmt.Sprintf("Promotion plan for %s (run %s) targeting %s branch %s.%s\n\nFiles:\n%s", proposal.Environment, proposal.RunID, proposal.GitOpsRepo.ProjectPath, proposal.Branch, descriptionSuffix(proposal.FileChanges), strings.Join(filePreviews(proposal.FileChanges), "\n"))
	source := fmt.Sprintf("%s/%s", proposal.Branch, proposal.RunID)
	return PromotionMergeRequestPayload{
		ProjectPath:  proposal.GitOpsRepo.ProjectPath,
		Title:        title,
		Description:  desc,
		SourceBranch: source,
		TargetBranch: proposal.Branch,
		Labels:       []string{"autodev-promotion", fmt.Sprintf("env:%s", environment)},
	}
}

func componentNames(list []ComponentMergeRequest) []string {
	names := make([]string, 0, len(list))
	for _, c := range list {
		names = append(names, c.Name)
	}
	return names
}

func filePreviews(files []PromotionFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, fmt.Sprintf("- %s: %s", file.Path, file.Preview))
	}
	return out
}

func descriptionSuffix(files []PromotionFile) string {
	if len(files) == 0 {
		return ""
	}
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return fmt.Sprintf(" Files: %s", strings.Join(paths, ", "))
}
