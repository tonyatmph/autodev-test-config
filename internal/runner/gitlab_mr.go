package runner

import (
	"fmt"

	"g7.mph.tech/mph-tech/autodev/internal/gitlab"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func (e *StageExecutor) publishImplementationMergeRequests(issue model.DeliveryIssue, run model.RunRequest, proposal gitlab.MergeRequestProposal) ([]gitlab.MergeRequest, error) {
	if e.gitlab == nil {
		return nil, nil
	}
	requests := gitlab.BuildImplementationMRRequests(proposal)
	created := make([]gitlab.MergeRequest, 0, len(requests))
	for _, request := range requests {
		if request.ProjectPath == "" {
			continue
		}
		mr, err := e.gitlab.UpsertMergeRequest(request.ProjectPath, request.Request)
		if err != nil {
			return nil, fmt.Errorf("upsert implementation merge request for %s: %w", request.Component.Name, err)
		}
		created = append(created, mr)
	}
	return created, nil
}

func (e *StageExecutor) publishPromotionMergeRequest(projectPath string, payload gitlab.PromotionMergeRequestPayload) (*gitlab.MergeRequest, error) {
	if e.gitlab == nil || projectPath == "" {
		return nil, nil
	}
	mr, err := e.gitlab.UpsertPromotionMergeRequest(projectPath, payload)
	if err != nil {
		return nil, fmt.Errorf("upsert promotion merge request for %s: %w", projectPath, err)
	}
	return &mr, nil
}
