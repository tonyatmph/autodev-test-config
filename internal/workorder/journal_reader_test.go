package workorder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestJournalReaderReadsStageFiles(t *testing.T) {
	root := t.TempDir()
	run := model.RunRequest{
		ID: "run-123",
		WorkOrder: model.WorkOrder{
			ID: "wo-abc",
		},
	}
	attempt := model.StageAttempt{
		Stage:   "test",
		Attempt: 1,
	}
	stageDir := filepath.Join(root, "work-orders", "wo-abc", "runs", "run-123", "stages", "test", "attempt-01")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	summary := `{"stage":"test","summary":"ok"}`
	report := `{"stage":"test","outputs":{"foo":"bar"}}`
	if err := os.WriteFile(filepath.Join(stageDir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	reader := NewReader(root)
	summaryPayload, err := reader.ReadSummary(run, attempt)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if summaryPayload["summary"] != "ok" {
		t.Fatalf("unexpected summary payload: %#v", summaryPayload)
	}
	reportPayload, err := reader.ReadReport(run, attempt)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if outputs, ok := reportPayload["outputs"].(map[string]any); !ok || outputs["foo"] != "bar" {
		t.Fatalf("unexpected report payload: %#v", reportPayload)
	}
}

func TestWritePipelineArtifacts(t *testing.T) {
	root := t.TempDir()
	run := model.RunRequest{
		ID: "run-123",
		WorkOrder: model.WorkOrder{
			ID: "wo-abc",
		},
	}
	written, err := WritePipelineArtifacts(root, run, map[string]any{
		"pipeline_intent":         validPipelineIntent(),
		"pipeline_execution_plan": validPipelineExecutionPlan(),
	})
	if err != nil {
		t.Fatalf("write pipeline artifacts: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 written files, got %v", written)
	}
	for _, rel := range written {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	paths := PipelineArtifactPaths(run)
	if !strings.HasSuffix(paths["pipeline_execution_plan"], "/pipeline_execution_plan.json") {
		t.Fatalf("unexpected pipeline artifact paths: %v", paths)
	}
}

func TestJournalWriterPersistsPipelineArtifacts(t *testing.T) {
	root := t.TempDir()
	run := model.RunRequest{
		ID: "run-xyz",
		WorkOrder: model.WorkOrder{
			ID: "wo-pipeline",
		},
	}
	writer := NewWriter(root)
	intent := validPipelineIntent()
	if err := writer.WritePipelineArtifact(run, "pipeline_intent", intent); err != nil {
		t.Fatalf("write intent: %v", err)
	}
	reader := NewReader(root)
	readIntent, err := reader.ReadPipelineArtifact(run, "pipeline_intent")
	if err != nil {
		t.Fatalf("read intent: %v", err)
	}
	if readIntent["schema_version"] != "autodev-pipeline-intent-v1" {
		t.Fatalf("unexpected intent payload: %#v", readIntent)
	}
}

func TestWritePipelineArtifactsRejectsMalformedExecutionPlan(t *testing.T) {
	root := t.TempDir()
	run := model.RunRequest{ID: "run-bad", WorkOrder: model.WorkOrder{ID: "wo-bad"}}
	_, err := WritePipelineArtifacts(root, run, map[string]any{
		"pipeline_execution_plan": map[string]any{
			"schema_version": "autodev-pipeline-execution-plan-v1",
			"run_id":         "run-bad",
			"issue_id":       "issue-bad",
		},
	})
	if err == nil {
		t.Fatal("expected malformed pipeline execution plan write to fail")
	}
}

func validPipelineIntent() map[string]any {
	return map[string]any{
		"schema_version":      "autodev-pipeline-intent-v1",
		"run_id":              "run-123",
		"issue_id":            "issue-123",
		"work_order_id":       "wo-abc",
		"issue_type":          "bug_fix",
		"pipeline_family":     "bugfix",
		"pipeline_selection":  "selected",
		"policy_profile":      "default",
		"delivery_name":       "demo",
		"requested_outcome":   "fix bug",
		"deploy_as_unit":      false,
		"selected_components": []string{"api"},
		"ordered_components":  []string{"api"},
		"documentation":       map[string]any{"required": false, "docs_component": ""},
		"testing":             map[string]any{"strategy": "tests-before-implementation", "immutable": true, "readable_by_agent": false, "executable_by_agent": true},
		"environments":        []map[string]any{{"name": "local", "project_path": "group/gitops", "path": "clusters/local/demo", "cluster": "local", "approval_required": false}},
	}
}

func validPolicyEvaluation() map[string]any {
	return map[string]any{
		"schema_version": "autodev-policy-evaluation-v1",
		"run_id":         "run-123",
		"issue_id":       "issue-123",
		"work_order_id":  "wo-abc",
		"policy_profile": "default",
		"outcome":        "approved",
		"pipeline_scope": []map[string]any{},
		"hierarchy":      []map[string]any{},
		"stage_scope":    map[string]any{},
		"stage_policies": []map[string]any{},
	}
}

func validPipelineBuildPlan() map[string]any {
	return map[string]any{
		"schema_version":           "autodev-pipeline-build-plan-v1",
		"run_id":                   "run-123",
		"issue_id":                 "issue-123",
		"work_order_id":            "wo-abc",
		"issue_type":               "bug_fix",
		"pipeline_family":          "bugfix",
		"delivery_name":            "demo",
		"images": []map[string]any{{
			"stage":        "plan",
			"image_ref":    "autodev-stage-plan:current",
			"image_digest": "sha256:plan",
			"container":    map[string]any{},
			"entrypoint":   []string{"autodev-stage-runtime"},
			"tooling_repo": map[string]any{"url": "tooling/plan", "ref": "main"},
			"prompt_file":  "prompts/plan.md",
		}},
	}
}

func validPipelineExecutionPlan() map[string]any {
	return map[string]any{
		"schema_version":           "autodev-pipeline-execution-plan-v1",
		"run_id":                   "run-123",
		"issue_id":                 "issue-123",
		"work_order_id":            "wo-abc",
		"issue_type":               "bug_fix",
		"pipeline_family":          "bugfix",
		"pipeline_selection":       "selected",
		"testing":                  map[string]any{"strategy": "tests-before-implementation", "immutable": true, "readable_by_agent": false, "executable_by_agent": true},
		"delivery_name":            "demo",
		"stages": []map[string]any{{
			"name":             "plan",
			"dependencies":     []string{},
			"queue_mode":       "auto",
			"transitions":      map[string]any{},
			"container":        map[string]any{},
			"success_criteria": map[string]any{},
			"output_artifacts": []map[string]any{},
			"report_stages":    []string{},
			"image_ref":        "autodev-stage-plan:current",
			"image_digest":     "sha256:plan",
			"entrypoint":       []string{"autodev-stage-runtime"},
		}},
	}
}
