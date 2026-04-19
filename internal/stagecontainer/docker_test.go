package stagecontainer

import (
	"os"
	"path/filepath"
	"testing"

	"g7.mph.tech/mph-tech/autodev/internal/app"
	"g7.mph.tech/mph-tech/autodev/internal/contracts"
	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func TestDockerArgsUseSpecEntrypoint(t *testing.T) {
	dataDir := t.TempDir()
	env := app.Env{
		DataDir:       dataDir,
		RootDir:       "/app",
		WorkOrderRepo: filepath.Join(dataDir, "work-orders"),
		RepoRoots:     []string{"/tmp/repos"},
	}
	spec := model.StageSpec{
		Name:       "plan",
		Entrypoint: []string{"autodev-stage-runtime", "--strict"},
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				RuntimeUser: model.RuntimeUserSpec{
					Mode:          model.RuntimeIsolationModeContainer,
					ContainerUser: "agent",
				},
			},
		},
	}

	workspace := filepath.Join(env.DataDir, "workspaces", "run-1", "plan")
	writeStageContext(t, filepath.Join(workspace, "context.json"), map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage":          map[string]any{},
		"run":            map[string]any{},
		"attempt":        map[string]any{},
		"issue":          map[string]any{},
		"work_order":     map[string]any{},
		"policy":         map[string]any{},
		"invariants":     map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images":   map[string]any{},
		},
		"materialized_repos": []map[string]any{
			{
				"type": "component",
				"path": filepath.Join(env.DataDir, "repos", "run-1", "component-a"),
			},
		},
		"runtime_isolation": map[string]any{
			"writable": []map[string]any{
				{
					"root":      filepath.Join(env.DataDir, "repos", "run-1", "component-a"),
					"patterns":  []string{"src/**"},
					"allow_all": false,
				},
			},
		},
		"paths": map[string]any{
			"work_order_repo": env.WorkOrderRepo,
		},
	})

	args, err := dockerArgs(env, spec, "run-1", "registry.local/stage-plan@sha256:abc")
	if err != nil {
		t.Fatalf("docker args: %v", err)
	}

	want := []string{
		"run",
		"--rm",
		"-w",
		"/app",
		"--user",
		"agent",
		"-e",
		"AUTODEV_STAGE_CONTEXT=" + filepath.Join(env.DataDir, "workspaces", "run-1", "plan", "context.json"),
		"-e",
		"AUTODEV_STAGE_RESULT=" + filepath.Join(env.DataDir, "workspaces", "run-1", "plan", "result.json"),
		"-e",
		"AUTODEV_STAGE_REPORT=" + filepath.Join(env.DataDir, "workspaces", "run-1", "plan", "report.json"),
		"-e",
		"AUTODEV_STAGE_STATE=" + filepath.Join(env.DataDir, "workspaces", "run-1", "plan", "state.json"),
		"--entrypoint",
		"/usr/local/bin/autodev-stage-runtime",
		"registry.local/stage-plan@sha256:abc",
		"--strict",
	}
	for _, token := range want {
		if !contains(args, token) {
			t.Fatalf("expected docker args to contain %q, got %v", token, args)
		}
	}

	if contains(args, "-v") && contains(args, env.DataDir+":"+env.DataDir) {
		t.Fatalf("did not expect broad data-dir mount in args: %v", args)
	}

	mountToken := "-v"
	expectedMounts := []string{
		filepath.Join(env.DataDir, "workspaces", "run-1", "plan") + ":" + filepath.Join(env.DataDir, "workspaces", "run-1", "plan"),
		filepath.Join(env.DataDir, "repos", "run-1", "component-a") + ":" + filepath.Join(env.DataDir, "repos", "run-1", "component-a") + ":ro",
		filepath.Join(env.DataDir, "repos", "run-1", "component-a", "src") + ":" + filepath.Join(env.DataDir, "repos", "run-1", "component-a", "src"),
		env.WorkOrderRepo + ":" + env.WorkOrderRepo + ":ro",
	}
	for _, expected := range expectedMounts {
		if !containsAdjacent(args, mountToken, expected) {
			t.Fatalf("expected mount %q in args %v", expected, args)
		}
	}
}

func TestComponentMountIsReadOnlyWhenStageLacksWritePermissions(t *testing.T) {
	dataDir := t.TempDir()
	env := app.Env{DataDir: dataDir, RootDir: "/app"}
	spec := model.StageSpec{
		Name:       "security",
		Entrypoint: []string{"autodev-stage-runtime"},
		Container:  model.StageContainer{},
	}
	workspace := filepath.Join(env.DataDir, "workspaces", "run-2", "security")
	repoPath := filepath.Join(env.DataDir, "repos", "run-2", "component-a")
	writeStageContext(t, filepath.Join(workspace, "context.json"), map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage":          map[string]any{},
		"run":            map[string]any{},
		"attempt":        map[string]any{},
		"issue":          map[string]any{},
		"work_order":     map[string]any{},
		"policy":         map[string]any{},
		"invariants":     map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images":   map[string]any{},
		},
		"materialized_repos": []map[string]any{{"type": "component", "path": repoPath}},
		"paths":              map[string]any{},
	})

	mountSet := mounts(env, spec, "run-2")
	expected := repoPath + ":" + repoPath + ":ro"
	if !contains(mountSet, expected) {
		t.Fatalf("expected read-only component mount %q, got %v", expected, mountSet)
	}
}

func TestComponentMountOnlyMakesDeclaredMutableSubpathsWritable(t *testing.T) {
	dataDir := t.TempDir()
	env := app.Env{DataDir: dataDir, RootDir: "/app"}
	spec := model.StageSpec{
		Name:       "implement",
		Entrypoint: []string{"autodev-stage-runtime"},
		Container: model.StageContainer{
			Permissions: model.StagePermissions{
				Writable:    []string{string(model.StageSurfaceComponents)},
				RepoControl: []string{string(model.StageSurfaceComponents)},
			},
		},
	}
	workspace := filepath.Join(env.DataDir, "workspaces", "run-3", "implement")
	repoPath := filepath.Join(env.DataDir, "repos", "run-3", "component-a")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("create git metadata dir: %v", err)
	}
	writeStageContext(t, filepath.Join(workspace, "context.json"), map[string]any{
		"schema_version": "autodev-stage-context-v1",
		"stage":          map[string]any{},
		"run":            map[string]any{},
		"attempt":        map[string]any{},
		"issue":          map[string]any{},
		"work_order":     map[string]any{},
		"policy":         map[string]any{},
		"invariants":     map[string]any{},
		"pipeline_contract": map[string]any{
			"stage_catalog":    []any{},
			"pipeline_catalog": map[string]any{},
			"runtime_images":   map[string]any{},
		},
		"runtime_isolation": map[string]any{
			"writable": []map[string]any{
				{
					"root":      repoPath,
					"patterns":  []string{"src/**", "docs/guides/**"},
					"allow_all": false,
				},
			},
			"repo_control": []map[string]any{
				{"root": repoPath},
			},
		},
		"materialized_repos": []map[string]any{{"type": "component", "path": repoPath}},
		"paths":              map[string]any{},
	})

	mountSet := mounts(env, spec, "run-3")
	if !contains(mountSet, repoPath+":"+repoPath+":ro") {
		t.Fatalf("expected repo root to remain read-only, got %v", mountSet)
	}
	for _, expected := range []string{
		filepath.Join(repoPath, "src") + ":" + filepath.Join(repoPath, "src"),
		filepath.Join(repoPath, "docs", "guides") + ":" + filepath.Join(repoPath, "docs", "guides"),
		filepath.Join(repoPath, ".git") + ":" + filepath.Join(repoPath, ".git"),
	} {
		if !contains(mountSet, expected) {
			t.Fatalf("expected writable subpath mount %q, got %v", expected, mountSet)
		}
	}
	if contains(mountSet, filepath.Join(repoPath, "policy")+":"+filepath.Join(repoPath, "policy")) {
		t.Fatalf("did not expect undeclared writable mount, got %v", mountSet)
	}
}

func writeStageContext(t *testing.T, path string, value any) {
	t.Helper()
	if err := contracts.WriteFile(path, contracts.StageContextSchema, value); err != nil {
		t.Fatalf("write stage context: %v", err)
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsAdjacent(values []string, left, right string) bool {
	for i := 0; i < len(values)-1; i++ {
		if values[i] == left && values[i+1] == right {
			return true
		}
	}
	return false
}
