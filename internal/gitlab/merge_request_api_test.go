package gitlab

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemAdapterUpsertMergeRequest(t *testing.T) {
	root := t.TempDir()
	adapter := NewFilesystemAdapter(root)
	mr, err := adapter.UpsertMergeRequest("mph-tech/example", MergeRequestCreatePayload{
		Title:        "Implement example",
		Description:  "example body",
		SourceBranch: "autodev/run-1/api",
		TargetBranch: "main",
		Labels:       []string{"autodev-implement"},
		RemoveSource: true,
	})
	if err != nil {
		t.Fatalf("upsert merge request: %v", err)
	}
	if mr.IID != 1 {
		t.Fatalf("expected iid 1, got %d", mr.IID)
	}
	path := filepath.Join(root, "merge_requests", "mph-tech-example", "autodev-run-1-api__main.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stored merge request: %v", err)
	}
	var stored MergeRequest
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("decode stored merge request: %v", err)
	}
	if stored.SourceBranch != "autodev/run-1/api" || stored.TargetBranch != "main" {
		t.Fatalf("unexpected stored merge request: %+v", stored)
	}
}

func TestAPIAdapterUpsertMergeRequestCreatesAndUpdates(t *testing.T) {
	var createCalls, updateCalls int
	adapter := NewAPIAdapter("https://gitlab.test", "token", "issues/project")
	adapter.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/projects/mph-tech/example/merge_requests":
			if updateCalls == 0 && createCalls == 0 {
				body = "[]"
				break
			}
			body = `[{"iid":7,"title":"Implement example","description":"updated","source_branch":"autodev/run-1/api","target_branch":"main","labels":["autodev-implement"],"state":"opened","web_url":"https://gitlab/mr/7"}]`
		case r.Method == http.MethodPost && r.URL.Path == "/projects/mph-tech/example/merge_requests":
			createCalls++
			body = `{"iid":7,"title":"Implement example","description":"created","source_branch":"autodev/run-1/api","target_branch":"main","labels":["autodev-implement"],"state":"opened","web_url":"https://gitlab/mr/7"}`
		case r.Method == http.MethodPut && r.URL.Path == "/projects/mph-tech/example/merge_requests/7":
			updateCalls++
			body = `{"iid":7,"title":"Implement example","description":"updated","source_branch":"autodev/run-1/api","target_branch":"main","labels":["autodev-implement"],"state":"opened","web_url":"https://gitlab/mr/7"}`
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	payload := MergeRequestCreatePayload{
		Title:        "Implement example",
		Description:  "body",
		SourceBranch: "autodev/run-1/api",
		TargetBranch: "main",
		Labels:       []string{"autodev-implement"},
		RemoveSource: true,
	}
	mr, err := adapter.UpsertMergeRequest("mph-tech/example", payload)
	if err != nil {
		t.Fatalf("create merge request: %v", err)
	}
	if mr.IID != 7 || createCalls != 1 {
		t.Fatalf("expected create call and iid 7, got %+v createCalls=%d", mr, createCalls)
	}
	mr, err = adapter.UpsertMergeRequest("mph-tech/example", payload)
	if err != nil {
		t.Fatalf("update merge request: %v", err)
	}
	if mr.IID != 7 || updateCalls != 1 {
		t.Fatalf("expected update call and iid 7, got %+v updateCalls=%d", mr, updateCalls)
	}
}
