package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteDoesNotLeakAmbientEnvToStageCommands(t *testing.T) {
	t.Setenv("LEAK_ME", "top-secret")
	t.Setenv("PYTHONPATH", "/tmp/leaky-pythonpath")
	t.Setenv("HOME", "/tmp/leaky-home")
	t.Setenv("PATH", "/tmp/leaky-path")
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	statePath := filepath.Join(dir, "state.json")
	writeJSON(t, contextPath, validContext(map[string]any{
		"stage": map[string]any{
			"name": "test",
			"operation_plan": map[string]any{
				"steps": []map[string]any{
					{
						"name": "check_env",
						"command": []string{
							"/bin/sh", "-c",
							`p=$AUTODEV_STAGE_METADATA_WORKING; d=$(cat $p); echo "${d%\}*}, \"leaked\":\"${LEAK_ME}\", \"pythonpath\":\"${PYTHONPATH}\", \"home\":\"${HOME}\", \"path\":\"${PATH}\"}" > $p`,
						},
					},
				},
			},
			"runtime": map[string]any{
				"summary": "ok",
				"transitions": map[string]any{
					"on_success": []string{},
					"on_failure": []string{},
				},
				"success_criteria": map[string]any{},
			},
		},
	}))

	result, report := execute(contextPath, statePath)
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s (%s)", result.Status, result.Summary)
	}
	if leaked, _ := report.Metadata["leaked"].(string); leaked != "" {
		t.Fatalf("expected ambient env to be filtered, got %q", leaked)
	}
	if pythonpath, _ := report.Metadata["pythonpath"].(string); pythonpath != "" {
		t.Fatalf("expected PYTHONPATH to be stripped, got %q", pythonpath)
	}
	if home, _ := report.Metadata["home"].(string); home != "" {
		t.Fatalf("expected HOME to be stripped, got %q", home)
	}
	if path, _ := report.Metadata["path"].(string); path == "/tmp/leaky-path" || path == "" {
		t.Fatalf("expected PATH to be fixed by runtime, got %q", path)
	}
}

func TestExecuteAttributesInvalidContractFailures(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	writeJSON(t, contextPath, map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage":          map[string]any{"name": "implement"},
		"run":            map[string]any{"id": "run-42"},
		"attempt":        map[string]any{"id": "attempt-7", "attempt": 3},
		"issue":          map[string]any{"id": "issue-9"},
		"policy":         map[string]any{},
		"invariants":     map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images":   map[string]any{},
		},
		"paths": map[string]any{},
	})

	result, report := execute(contextPath, "")
	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %s", result.Status)
	}
	if report.RunID != "run-42" || report.IssueID != "issue-9" || report.Stage != "implement" || report.Attempt != 3 {
		t.Fatalf("expected attributed report, got %+v", report)
	}
}

func TestExecuteFailsOnMalformedWorkingResultJSON(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	statePath := filepath.Join(dir, "state.json")
	writeJSON(t, contextPath, validContext(map[string]any{
		"stage": map[string]any{
			"name": "test",
			"operation_plan": map[string]any{
				"steps": []map[string]any{
					{
						"name": "corrupt_result",
						"command": []string{
							"sh", "-c",
							`echo "{not-json" > "$AUTODEV_STAGE_RESULT_WORKING"`,
						},
					},
				},
			},
			"runtime": map[string]any{
				"summary": "ok",
				"transitions": map[string]any{
					"on_success": []string{},
					"on_failure": []string{"failed-stage"},
				},
				"success_criteria": map[string]any{},
			},
		},
	}))

	result, report := execute(contextPath, statePath)
	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %s", result.Status)
	}
	if report.Status != "failed" {
		t.Fatalf("expected failed report, got %s", report.Status)
	}
	if result.Summary == "" || report.Summary == "" {
		t.Fatalf("expected failure summary, got result=%q report=%q", result.Summary, report.Summary)
	}
	if result.NextSignals == nil || len(result.NextSignals) != 1 || result.NextSignals[0] != "failed-stage" {
		t.Fatalf("expected failure transition signals, got %+v", result.NextSignals)
	}
}

func validContext(overrides map[string]any) map[string]any {
	ctx := map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage": map[string]any{
			"name": "test",
			"operation_plan": map[string]any{
				"steps": []any{},
			},
			"runtime": map[string]any{
				"summary": "ok",
				"transitions": map[string]any{
					"on_success": []string{},
					"on_failure": []string{},
				},
				"success_criteria": map[string]any{},
			},
		},
		"run":        map[string]any{"id": "run-1"},
		"attempt":    map[string]any{"id": "attempt-1", "attempt": 1},
		"issue":      map[string]any{"id": "issue-1"},
		"work_order": map[string]any{"id": "wo-1"},
		"policy":     map[string]any{},
		"invariants": map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images":   map[string]any{},
		},
		"paths": map[string]any{},
	}
	for key, value := range overrides {
		ctx[key] = value
	}
	return ctx
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
