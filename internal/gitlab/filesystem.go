package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Adapter interface {
	ListIssues() ([]model.DeliveryIssue, error)
	AppendComment(issueID, body string) error
	SetLabels(issueID string, labels []string) error
	UpsertMergeRequest(projectPath string, payload MergeRequestCreatePayload) (MergeRequest, error)
	UpsertPromotionMergeRequest(projectPath string, payload PromotionMergeRequestPayload) (MergeRequest, error)
}

type FilesystemAdapter struct {
	root string
}

func NewFilesystemAdapter(root string) *FilesystemAdapter {
	return &FilesystemAdapter{root: root}
}

func (a *FilesystemAdapter) ListIssues() ([]model.DeliveryIssue, error) {
	dir := filepath.Join(a.root, "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create gitlab issues dir: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read issues dir: %w", err)
	}

	issues := make([]model.DeliveryIssue, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read issue %s: %w", path, err)
		}
		var issue model.DeliveryIssue
		if err := json.Unmarshal(data, &issue); err != nil {
			return nil, fmt.Errorf("decode issue %s: %w", path, err)
		}
		issues = append(issues, issue)
	}

	sort.Slice(issues, func(i, j int) bool {
		return issues[i].ID < issues[j].ID
	})
	return issues, nil
}

func (a *FilesystemAdapter) AppendComment(issueID, body string) error {
	issue, path, err := a.readIssue(issueID)
	if err != nil {
		return err
	}

	if issue.Metadata == nil {
		issue.Metadata = map[string]any{}
	}
	var comments []map[string]any
	if raw, ok := issue.Metadata["comments"]; ok {
		encoded, _ := json.Marshal(raw)
		_ = json.Unmarshal(encoded, &comments)
	}
	comments = append(comments, map[string]any{
		"id":         fmt.Sprintf("%s-comment-%d", issueID, len(comments)+1),
		"body":       body,
		"created_at": time.Now().UTC(),
	})
	issue.Metadata["comments"] = comments
	issue.UpdatedAt = time.Now().UTC()
	return writeJSON(path, issue)
}

func (a *FilesystemAdapter) SetLabels(issueID string, labels []string) error {
	issue, path, err := a.readIssue(issueID)
	if err != nil {
		return err
	}
	issue.Labels = labels
	issue.UpdatedAt = time.Now().UTC()
	return writeJSON(path, issue)
}

func (a *FilesystemAdapter) UpsertMergeRequest(projectPath string, payload MergeRequestCreatePayload) (MergeRequest, error) {
	return a.upsertMergeRequest(projectPath, payload.Title, payload.Description, payload.SourceBranch, payload.TargetBranch, payload.Labels, payload.RemoveSource)
}

func (a *FilesystemAdapter) UpsertPromotionMergeRequest(projectPath string, payload PromotionMergeRequestPayload) (MergeRequest, error) {
	return a.upsertMergeRequest(projectPath, payload.Title, payload.Description, payload.SourceBranch, payload.TargetBranch, payload.Labels, false)
}

func (a *FilesystemAdapter) upsertMergeRequest(projectPath, title, description, sourceBranch, targetBranch string, labels []string, removeSource bool) (MergeRequest, error) {
	root := filepath.Join(a.root, "merge_requests", sanitizeProjectPath(projectPath))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return MergeRequest{}, fmt.Errorf("create merge request dir: %w", err)
	}
	key := sanitizeProjectPath(sourceBranch) + "__" + sanitizeProjectPath(targetBranch) + ".json"
	path := filepath.Join(root, key)
	var mr MergeRequest
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &mr); err != nil {
			return MergeRequest{}, fmt.Errorf("decode merge request %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return MergeRequest{}, fmt.Errorf("read merge request %s: %w", path, err)
	}
	if mr.IID == 0 {
		next, err := a.nextMergeRequestIID(root)
		if err != nil {
			return MergeRequest{}, err
		}
		mr.IID = next
	}
	mr.ProjectPath = projectPath
	mr.Title = title
	mr.Description = description
	mr.SourceBranch = sourceBranch
	mr.TargetBranch = targetBranch
	mr.Labels = append([]string(nil), labels...)
	mr.RemoveSourceBranch = removeSource
	mr.State = "opened"
	mr.WebURL = fmt.Sprintf("file://%s", path)
	if err := writeJSON(path, mr); err != nil {
		return MergeRequest{}, err
	}
	return mr, nil
}

func (a *FilesystemAdapter) readIssue(issueID string) (model.DeliveryIssue, string, error) {
	path := filepath.Join(a.root, "issues", issueID+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return model.DeliveryIssue{}, "", fmt.Errorf("issue %s not found", issueID)
	}
	if err != nil {
		return model.DeliveryIssue{}, "", fmt.Errorf("read issue %s: %w", issueID, err)
	}
	var issue model.DeliveryIssue
	if err := json.Unmarshal(data, &issue); err != nil {
		return model.DeliveryIssue{}, "", fmt.Errorf("decode issue %s: %w", issueID, err)
	}
	return issue, path, nil
}

func (a *FilesystemAdapter) nextMergeRequestIID(root string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, fmt.Errorf("read merge request dir: %w", err)
	}
	maxIID := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("read merge request %s: %w", entry.Name(), err)
		}
		var mr MergeRequest
		if err := json.Unmarshal(data, &mr); err != nil {
			return 0, fmt.Errorf("decode merge request %s: %w", entry.Name(), err)
		}
		if mr.IID > maxIID {
			maxIID = mr.IID
		}
	}
	return maxIID + 1, nil
}

func sanitizeProjectPath(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "..", "-")
	return replacer.Replace(value)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
