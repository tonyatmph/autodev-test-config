package gitlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

var jsonFencePattern = regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")

type APIAdapter struct {
	baseURL       string
	token         string
	issuesProject string
	client        *http.Client
}

func NewAPIAdapter(baseURL, token, issuesProject string) *APIAdapter {
	return &APIAdapter{
		baseURL:       strings.TrimRight(baseURL, "/"),
		token:         token,
		issuesProject: issuesProject,
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

type apiIssue struct {
	IID         int      `json:"iid"`
	ProjectID   int      `json:"project_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	UpdatedAt   string   `json:"updated_at"`
}

func (a *APIAdapter) ListIssues() ([]model.DeliveryIssue, error) {
	issues := make([]model.DeliveryIssue, 0)
	page := 1
	for {
		var batch []apiIssue
		path := fmt.Sprintf("/projects/%s/issues?state=opened&per_page=100&page=%d", url.PathEscape(a.issuesProject), page)
		if err := a.get(path, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, issue := range batch {
			if !hasDeliveryLabel(issue.Labels) {
				continue
			}
			delivery, err := decodeAPIIssue(a.issuesProject, issue)
			if err != nil {
				return nil, err
			}
			issues = append(issues, delivery)
		}
		if len(batch) < 100 {
			break
		}
		page++
	}
	return issues, nil
}

func (a *APIAdapter) AppendComment(issueID, body string) error {
	projectID, iid, err := parseIssueID(issueID)
	if err != nil {
		return err
	}
	payload := map[string]string{"body": body}
	return a.postJSON(fmt.Sprintf("/projects/%s/issues/%d/notes", url.PathEscape(projectID), iid), payload, nil)
}

func (a *APIAdapter) SetLabels(issueID string, labels []string) error {
	projectID, iid, err := parseIssueID(issueID)
	if err != nil {
		return err
	}
	payload := map[string]string{"labels": strings.Join(labels, ",")}
	return a.putJSON(fmt.Sprintf("/projects/%s/issues/%d", url.PathEscape(projectID), iid), payload, nil)
}

func decodeAPIIssue(defaultProject string, issue apiIssue) (model.DeliveryIssue, error) {
	var spec model.DeliveryIssue
	if err := extractSpec(issue.Description, &spec); err != nil {
		return model.DeliveryIssue{}, err
	}
	projectID := strconv.Itoa(issue.ProjectID)
	if projectID == "0" {
		projectID = defaultProject
	}
	updatedAt, _ := time.Parse(time.RFC3339, issue.UpdatedAt)
	spec.ID = canonicalIssueID(projectID, issue.IID)
	spec.ProjectID = projectID
	spec.IID = issue.IID
	spec.Title = issue.Title
	spec.Description = issue.Description
	spec.Labels = append([]string(nil), issue.Labels...)
	spec.UpdatedAt = updatedAt
	if spec.Approval.Label == "" {
		spec.Approval.Label = "delivery/approved"
	}
	spec.Approval.Approved = slicesContains(issue.Labels, spec.Approval.Label)
	spec.WorkOrder = spec.CanonicalWorkOrder()
	spec.Target = spec.WorkOrder.Delivery
	spec.RequestedOutcome = spec.WorkOrder.RequestedOutcome
	spec.PolicyProfile = spec.WorkOrder.PolicyProfile
	spec.PipelineTemplate = spec.WorkOrder.PipelineTemplate
	if spec.Metadata == nil {
		spec.Metadata = map[string]any{}
	}
	return spec, nil
}

func extractSpec(description string, out any) error {
	matches := jsonFencePattern.FindStringSubmatch(description)
	if len(matches) != 2 {
		return fmt.Errorf("issue description does not contain a ```json delivery spec block")
	}
	payload := []byte(matches[1])
	if err := contracts.Validate(contracts.IssueSpecSchema, "gitlab issue delivery spec", payload); err != nil {
		return err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode issue delivery spec: %w", err)
	}
	return nil
}

func canonicalIssueID(projectID string, iid int) string {
	return fmt.Sprintf("gitlab:%s:%d", projectID, iid)
}

func parseIssueID(value string) (string, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || parts[0] != "gitlab" {
		return "", 0, fmt.Errorf("invalid gitlab issue id %q", value)
	}
	iid, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, fmt.Errorf("parse issue iid: %w", err)
	}
	return parts[1], iid, nil
}

func (a *APIAdapter) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return err
	}
	return a.do(req, out)
}

func (a *APIAdapter) postJSON(path string, body any, out any) error {
	return a.sendJSON(http.MethodPost, path, body, out)
}

func (a *APIAdapter) putJSON(path string, body any, out any) error {
	return a.sendJSON(http.MethodPut, path, body, out)
}

func (a *APIAdapter) sendJSON(method, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, a.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, out)
}

func (a *APIAdapter) do(req *http.Request, out any) error {
	req.Header.Set("PRIVATE-TOKEN", a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitlab api %s %s failed: %s", req.Method, req.URL.Path, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasDeliveryLabel(labels []string) bool {
	for _, label := range labels {
		if strings.HasPrefix(label, "delivery/") {
			return true
		}
	}
	return false
}
