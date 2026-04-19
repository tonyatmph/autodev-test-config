package gitlab

import (
	"fmt"
	"net/url"
)

type MergeRequest struct {
	ProjectPath        string   `json:"project_path"`
	IID                int      `json:"iid"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	SourceBranch       string   `json:"source_branch"`
	TargetBranch       string   `json:"target_branch"`
	Labels             []string `json:"labels,omitempty"`
	State              string   `json:"state"`
	WebURL             string   `json:"web_url,omitempty"`
	RemoveSourceBranch bool     `json:"remove_source_branch,omitempty"`
}

type apiMergeRequest struct {
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	Labels       []string `json:"labels"`
	State        string   `json:"state"`
	WebURL       string   `json:"web_url"`
}

func (a *APIAdapter) UpsertMergeRequest(projectPath string, payload MergeRequestCreatePayload) (MergeRequest, error) {
	return a.upsertMergeRequest(projectPath, payload.Title, payload.Description, payload.SourceBranch, payload.TargetBranch, payload.Labels, payload.RemoveSource)
}

func (a *APIAdapter) UpsertPromotionMergeRequest(projectPath string, payload PromotionMergeRequestPayload) (MergeRequest, error) {
	return a.upsertMergeRequest(projectPath, payload.Title, payload.Description, payload.SourceBranch, payload.TargetBranch, payload.Labels, false)
}

func (a *APIAdapter) upsertMergeRequest(projectPath, title, description, sourceBranch, targetBranch string, labels []string, removeSource bool) (MergeRequest, error) {
	existing, err := a.findOpenMergeRequest(projectPath, sourceBranch, targetBranch)
	if err != nil {
		return MergeRequest{}, err
	}
	if existing != nil {
		payload := map[string]any{
			"title":        title,
			"description":  description,
			"target_branch": targetBranch,
			"labels":       labels,
		}
		var updated apiMergeRequest
		path := fmt.Sprintf("/projects/%s/merge_requests/%d", url.PathEscape(projectPath), existing.IID)
		if err := a.putJSON(path, payload, &updated); err != nil {
			return MergeRequest{}, err
		}
		return mergeRequestFromAPI(projectPath, updated, removeSource), nil
	}
	payload := map[string]any{
		"title":                title,
		"description":          description,
		"source_branch":        sourceBranch,
		"target_branch":        targetBranch,
		"labels":               labels,
		"remove_source_branch": removeSource,
	}
	var created apiMergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests", url.PathEscape(projectPath))
	if err := a.postJSON(path, payload, &created); err != nil {
		return MergeRequest{}, err
	}
	return mergeRequestFromAPI(projectPath, created, removeSource), nil
}

func (a *APIAdapter) findOpenMergeRequest(projectPath, sourceBranch, targetBranch string) (*apiMergeRequest, error) {
	var list []apiMergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests?state=opened&source_branch=%s&target_branch=%s", url.PathEscape(projectPath), url.QueryEscape(sourceBranch), url.QueryEscape(targetBranch))
	if err := a.get(path, &list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

func mergeRequestFromAPI(projectPath string, mr apiMergeRequest, removeSource bool) MergeRequest {
	return MergeRequest{
		ProjectPath:        projectPath,
		IID:                mr.IID,
		Title:              mr.Title,
		Description:        mr.Description,
		SourceBranch:       mr.SourceBranch,
		TargetBranch:       mr.TargetBranch,
		Labels:             append([]string(nil), mr.Labels...),
		State:              mr.State,
		WebURL:             mr.WebURL,
		RemoveSourceBranch: removeSource,
	}
}
