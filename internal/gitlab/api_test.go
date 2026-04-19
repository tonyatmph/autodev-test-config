package gitlab

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAPIAdapterListIssuesDecodesDeliverySpec(t *testing.T) {
	spec := map[string]any{
		"approval": map[string]any{
			"label": "delivery/approved",
		},
		"work_order": map[string]any{
			"id":                "wo-42",
			"issue_type":        "new_feature",
			"requested_outcome": "ship it",
			"policy_profile":    "default",
			"pipeline_template": "default-v1",
			"issuer_authority": map[string]any{
				"can_create_pipeline": false,
				"roles":               []string{"developer"},
			},
			"testing": map[string]any{
				"strategy":            "governed-inspection",
				"immutable":           true,
				"readable_by_agent":   false,
				"executable_by_agent": true,
				"inspection_points": []map[string]any{
					{
						"name":                "security",
						"category":            "security",
						"description":         "Security testing is part of the delivery contract.",
						"immutable":           true,
						"readable_by_agent":   false,
						"executable_by_agent": true,
					},
				},
			},
			"translation": map[string]any{
				"translator": "test",
				"version":    "v1",
				"status":     "translated",
				"warnings":   []string{},
			},
			"delivery": map[string]any{
				"name":                "example-platform",
				"primary_component":   "api",
				"selected_components": []string{"api", "console"},
				"deploy_as_unit":      true,
				"documentation": map[string]any{
					"required":       false,
					"docs_component": "docs",
					"required_kinds": []string{},
				},
				"journal": map[string]any{
					"name": "work-orders",
					"repo": map[string]any{
						"project_path":          "work-orders",
						"default_branch":        "main",
						"working_branch_prefix": "autodev",
						"ref":                   "main",
						"materialization_path":  "data/repos/{run_id}/journal/work-orders",
					},
					"path":     "runs",
					"strategy": "git",
				},
				"components": map[string]any{
					"api": map[string]any{
						"name":       "example-api",
						"kind":       "api",
						"deployable": true,
						"repo": map[string]any{
							"project_path":          "mph-tech/example-service",
							"default_branch":        "main",
							"working_branch_prefix": "autodev",
							"ref":                   "main",
							"materialization_path":  "data/repos/{run_id}/components/api",
						},
						"release": map[string]any{
							"application": map[string]any{
								"artifact_name": "example-api",
								"image_repo":    "registry.mph.tech/example-api",
							},
						},
					},
					"console": map[string]any{
						"name":       "example-console",
						"kind":       "console",
						"deployable": true,
						"repo": map[string]any{
							"project_path":          "mph-tech/example-console",
							"default_branch":        "main",
							"working_branch_prefix": "autodev",
							"ref":                   "main",
							"materialization_path":  "data/repos/{run_id}/components/console",
						},
						"release": map[string]any{
							"application": map[string]any{
								"artifact_name": "example-console",
								"image_repo":    "registry.mph.tech/example-console",
							},
						},
					},
				},
				"environments": map[string]any{
					"dev": map[string]any{
						"name": "dev",
						"gitops_repo": map[string]any{
							"project_path":         "mph-tech/example-gitops",
							"environment":          "dev",
							"path":                 "clusters/dev/example-platform",
							"promotion_branch":     "main",
							"cluster":              "dev",
							"ref":                  "main",
							"materialization_path": "data/repos/{run_id}/gitops/dev",
						},
						"approval_required": false,
						"rollout_strategy":  "rolling",
					},
				},
				"release": map[string]any{
					"application": map[string]any{
						"artifact_name": "example-platform",
						"image_repo":    "registry.mph.tech/example-platform",
					},
				},
			},
		},
		"metadata": map[string]any{
			"tenant": "test",
		},
	}
	specData, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal issue spec: %v", err)
	}
	adapter := NewAPIAdapter("https://gitlab.test", "token-123", "platform/delivery")
	adapter.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/projects/platform/delivery/issues" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token-123" {
			t.Fatalf("missing token header")
		}
		payload, err := json.Marshal([]map[string]any{
			{
				"iid":         42,
				"project_id":  1234,
				"title":       "Example issue",
				"description": "Delivery request\n\n```json\n" + string(specData) + "\n```",
				"labels":      []string{"delivery/requested", "priority/high"},
				"updated_at":  "2026-04-13T12:00:00Z",
			},
		})
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(payload))),
		}, nil
	})}
	issues, err := adapter.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "gitlab:1234:42" {
		t.Fatalf("unexpected issue id: %s", issues[0].ID)
	}
	apiComponent, ok := issues[0].WorkOrder.Delivery.Component("api")
	if !ok {
		t.Fatalf("expected api component in work order delivery")
	}
	if apiComponent.Repo.ProjectPath != "mph-tech/example-service" {
		t.Fatalf("unexpected api repo: %+v", apiComponent.Repo)
	}
	if got := issues[0].WorkOrder.Delivery.SelectedComponents; len(got) != 2 || got[0] != "api" || got[1] != "console" {
		t.Fatalf("unexpected selected components: %+v", got)
	}
	if issues[0].Approval.Approved {
		t.Fatalf("expected approval to be false when approval label is missing, got %+v", issues[0].Approval)
	}
}

func TestAPIAdapterMutationsUseIssueID(t *testing.T) {
	var calls []string
	adapter := NewAPIAdapter("https://gitlab.test", "token-123", "platform/delivery")
	adapter.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	if err := adapter.AppendComment("gitlab:1234:42", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.SetLabels("gitlab:1234:42", []string{"delivery/active", "priority/high"}); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(calls, "\n")
	if !strings.Contains(got, "POST /projects/1234/issues/42/notes") {
		t.Fatalf("unexpected calls: %s", got)
	}
	if !strings.Contains(got, "PUT /projects/1234/issues/42") {
		t.Fatalf("unexpected calls: %s", got)
	}
}
